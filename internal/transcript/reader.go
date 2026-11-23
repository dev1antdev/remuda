package transcript

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/text"
)

// Reader reads the session files on one machine: in process here, by script
// over ssh with the remote tail cached and grown by delta. It also finds the
// file herdr's agent_session names and follows a conversation through the
// files a compaction moved it to.
type Reader struct {
	host shell.Host

	mu           sync.Mutex
	tails        map[string]*cachedTail // remote files by path
	sessionPath  map[sessionKey]string  // session id and root generation -> resolved path
	sessionAt    map[sessionKey]time.Time
	sessionMiss  map[sessionKey]time.Time
	continuation map[string]continuation // session file -> its continuation, as last looked
	predecessor  map[string]string       // session file -> the file it continues ("" when none)
	learnedRoots map[Format]map[string]time.Time
	active       map[string]activeSession
	rootVersion  uint64
	subTails     map[string]*cachedTail // remote subagent files by path
	subagents    map[string]*subagentEntry
	subagentUses uint64                // the use count behind subagentEntry.used
	subDirs      map[string]dirListing // a remote session's subagent folder, as listed a moment ago
	metas        map[string]cachedMeta // the subagents' meta files as read, by path
}

type cachedTail struct {
	version Version
	data    []byte
	read    time.Time // when it was last asked for
}

type continuation struct {
	next string
	at   time.Time
}

type sessionKey struct {
	format      Format
	cwd, id     string
	rootVersion uint64
}

type activeSession struct {
	path  string
	until time.Time
}

const (
	remoteTailCap = 8 * 1024 * 1024 // the most of one remote file kept
	// what each cache of a machine's remote files holds at most: a wall of busy agents reads every file in turn
	remoteTailsBudget    = 128 << 20
	remoteSubTailsBudget = 64 << 20
	// a tail asked for this recently is not pushed out for a file not cached yet: the next read would fetch it whole again
	recentlyRead = time.Minute
	// how long a folder an agent named for itself is trusted after a process last named it
	learnedRootTTL = 10 * time.Minute
	// how long a Codex id's resolved file stands before it is looked up again: a revert writes a newer file at once
	codexResolveFresh = 2 * time.Second
)

// NewReader reads the transcripts on host.
func NewReader(host shell.Host) *Reader {
	return &Reader{host: host, tails: map[string]*cachedTail{}, sessionPath: map[sessionKey]string{}, sessionAt: map[sessionKey]time.Time{}, sessionMiss: map[sessionKey]time.Time{},
		continuation: map[string]continuation{}, predecessor: map[string]string{},
		learnedRoots: map[Format]map[string]time.Time{},
		active:       map[string]activeSession{},
		subTails:     map[string]*cachedTail{}, subagents: map[string]*subagentEntry{}, subDirs: map[string]dirListing{}, metas: map[string]cachedMeta{}}
}

// addRoot keeps a folder an agent named for itself after an accepted process read, for a while: its session is found
// under it until no accepted read renews it.
func (r *Reader) addRoot(f Format, dir string) {
	if f != Claude && f != Codex {
		return
	}
	style := rpath.For(r.host.Home())
	dir = style.Clean(dir)
	if dir == "" || !style.IsAbs(dir) {
		return
	}
	for _, fallback := range RootsOf(r.host.Home())[f] {
		if style.Equal(fallback, dir) {
			return
		}
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.learnedRoots[f] == nil {
		r.learnedRoots[f] = map[string]time.Time{}
	}
	until, known := r.learnedRoots[f][dir]
	r.learnedRoots[f][dir] = now.Add(learnedRootTTL)
	if !known || !until.After(now) {
		r.bumpRootsLocked()
	}
}

// bumpRootsLocked starts another root generation and drops what was resolved against the one before. The caller holds r.mu.
func (r *Reader) bumpRootsLocked() {
	r.rootVersion++
	for key := range r.sessionPath {
		if key.rootVersion != r.rootVersion {
			delete(r.sessionPath, key)
			delete(r.sessionAt, key)
		}
	}
	for key := range r.sessionMiss {
		if key.rootVersion != r.rootVersion {
			delete(r.sessionMiss, key)
		}
	}
}

// rememberSession keeps the file an accepted process read named, so a copy of the same id under another root cannot be
// opened before the agent's live one.
func (r *Reader) rememberSession(id, path string) {
	if id == "" || path == "" {
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for known, session := range r.active {
		if !session.until.After(now) {
			delete(r.active, known)
		}
	}
	r.active[id] = activeSession{path: path, until: now.Add(learnedRootTTL)}
}

// activePath is the live file an accepted process read named, while that reading is fresh.
func (r *Reader) activePath(id string) string {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.active[id]
	if !ok || !session.until.After(now) {
		delete(r.active, id)
		return ""
	}
	return session.path
}

// regularSession says a session path is a regular file on the machine.
func (r *Reader) regularSession(path string) bool {
	if r.host.Local() {
		return sessionFile(path)
	}
	_, err := shell.Text(r.host, "if "+remoteSessionFile(path)+"; then exit 0; else exit 1; fi", 10*time.Second)
	return err == nil
}

// Roots is this machine's session folders: its home's own and the ones accepted processes named for themselves while
// they are fresh. It is a copy: a scanner may keep it.
func (r *Reader) Roots() Roots {
	roots, _ := r.rootsSnapshot()
	return roots
}

// rootsSnapshot is Roots with the generation it was read at, so a resolved path is never cached against another set.
func (r *Reader) rootsSnapshot() (Roots, uint64) {
	roots, style := RootsOf(r.host.Home()), rpath.For(r.host.Home())
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	expired := false
	for f, learned := range r.learnedRoots {
		for dir, until := range learned {
			if !until.After(now) {
				delete(learned, dir)
				expired = true
				continue
			}
			known := false
			for _, fallback := range roots[f] {
				if style.Equal(fallback, dir) {
					known = true
					break
				}
			}
			if !known {
				roots[f] = append(roots[f], dir)
			}
		}
	}
	if expired {
		r.bumpRootsLocked()
	}
	return roots, r.rootVersion
}

// open opens a session file in the folder it belongs to: locally inside that root's own confinement, remotely as
// OpenSession checks it.
func (r *Reader) open(path string) (*os.File, error) {
	if r.host.Local() {
		style := rpath.For(r.host.Home())
		for _, dirs := range r.Roots() {
			for _, root := range dirs {
				if rel, ok := style.Rel(root, path); ok && rel != "." {
					return OpenInRoot(root, path)
				}
			}
		}
	}
	return OpenSession(path)
}

// Version stats the session file; the version doubles as the ETag.
func (r *Reader) Version(path string) (Version, error) {
	if r.host.Local() {
		f, err := r.open(path)
		if err != nil {
			return Version{}, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return Version{}, err
		}
		return Version{Size: info.Size(), MTime: info.ModTime().UnixNano()}, nil
	}
	out, err := shell.Text(r.host, script.Stat(script.Quote(path), "%s %Y", "%z %m"), 10*time.Second)
	if err != nil {
		return Version{}, err
	}
	var v Version
	if _, err := fmt.Sscanf(out, "%d %d", &v.Size, &v.MTime); err != nil {
		return Version{}, fmt.Errorf("stat %s: %q", path, out)
	}
	return v, nil
}

// DirVersions is the size and mtime of every .jsonl directly in dir, in one round trip; a missing dir is an empty map.
func (r *Reader) DirVersions(dir string) (map[string]Version, error) {
	return r.dirVersions(dir, ".jsonl")
}

// dirVersions is DirVersions for the files of the given suffixes.
func (r *Reader) dirVersions(dir string, suffixes ...string) (map[string]Version, error) {
	out := map[string]Version{}
	named := func(name string) bool {
		for _, suffix := range suffixes {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
		return false
	}
	if r.host.Local() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return out, nil
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !named(entry.Name()) {
				continue
			}
			if info, err := entry.Info(); err == nil {
				out[filepath.Join(dir, entry.Name())] = Version{Size: info.Size(), MTime: info.ModTime().UnixNano()}
			}
		}
		return out, nil
	}
	quoted := script.Quote(dir)
	globs := make([]string, len(suffixes))
	for i, suffix := range suffixes {
		globs[i] = quoted + "/*" + suffix
	}
	listing := `[ -d ` + quoted + ` ] || exit 0; for f in ` + strings.Join(globs, " ") + `; do [ -f "$f" ] || continue; ` + script.Stat(`"$f"`, "%s %Y %n", "%z %m %N") + `; done`
	raw, err := shell.Text(r.host, listing, 20*time.Second)
	if err != nil {
		return nil, err
	}
	for _, line := range script.Lines(raw) {
		var v Version
		rest := ""
		if n, _ := fmt.Sscanf(line, "%d %d %s", &v.Size, &v.MTime, &rest); n == 3 {
			// the path may hold spaces: everything after the two numbers
			parts := strings.SplitN(line, " ", 3)
			out[parts[2]] = v
		}
	}
	return out, nil
}

// ReadFrom returns a file's bytes from offset to its end; remotely a tail over ssh.
func (r *Reader) ReadFrom(path string, offset int64) ([]byte, error) {
	if r.host.Local() {
		f, err := r.open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, err
		}
		return io.ReadAll(io.LimitReader(f, 256<<20))
	}
	return r.host.Run(fmt.Sprintf("if %s; then tail -c +%d %s; else exit 1; fi", remoteSessionFile(path), offset+1, script.Quote(path)), 120*time.Second)
}

// remoteSessionFile is the shell test a remote read makes first: a regular file, never a link or a named pipe.
func remoteSessionFile(path string) string {
	q := script.Quote(path)
	return "[ -f " + q + " ] && [ ! -L " + q + " ]"
}

// Turns parses the session file at the given version, remotely from a cached tail grown by delta.
// readSession is a session file's whole content, local or remote, only from under the machine's session roots.
func (r *Reader) readSession(path string) ([]byte, error) {
	if !underRoots(r.Roots(), rpath.For(r.host.Home()), path) {
		return nil, fmt.Errorf("%s is outside the machine's session roots", path)
	}
	if r.host.Local() {
		f, err := r.open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, 256<<20))
	}
	return r.ReadFrom(path, 0)
}

// Turns parses the session file at the given version, remotely from a cached tail grown by delta.
func (r *Reader) Turns(path string, v Version) ([]Turn, error) {
	if FormatOf(path) == Codex {
		return parseCodexHistory(path, r.readSession) // a revert's history_base may lie in another root or on ssh
	}
	if r.host.Local() {
		f, err := r.open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		switch FormatOf(path) {
		case Gemini:
			data, err := io.ReadAll(io.LimitReader(f, tailWindow))
			if err != nil {
				return nil, err
			}
			return parseGemini(data)
		default:
			return parseRecords(f, false)
		}
	}
	data, err := r.remoteTail(path, v)
	if err != nil {
		return nil, err
	}
	return ParseBytesOf(FormatOf(path), data)
}

// remoteTail returns the file tail, fetching only the delta since the cached copy when the file merely grew.
func (r *Reader) remoteTail(path string, v Version) ([]byte, error) {
	return r.remoteTailIn(r.tails, path, v, remoteTailsBudget)
}

// remoteTailIn is remoteTail over one of the caches, which holds at most budget bytes (see keepTail).
func (r *Reader) remoteTailIn(tails map[string]*cachedTail, path string, v Version, budget int) ([]byte, error) {
	r.mu.Lock()
	cached := tails[path]
	if cached != nil && cached.version == v {
		cached.read = time.Now()
		r.mu.Unlock()
		return cached.data, nil
	}
	r.mu.Unlock()

	// every read is cut at the size the stat saw: what the agent wrote since belongs to the next version
	quoted := script.Quote(path)
	var data []byte
	if cached != nil && v.Size > cached.version.Size {
		// grew: fetch the delta only (tail -c +N is 1-indexed)
		delta, err := r.host.Run(fmt.Sprintf("if %s; then tail -c +%d %s | head -c %d; else exit 1; fi",
			remoteSessionFile(path), cached.version.Size+1, quoted, v.Size-cached.version.Size), 30*time.Second)
		if err == nil {
			data = append(cached.data[:len(cached.data):len(cached.data)], delta...) // the cached slice is shared: copy
		}
	}
	if data == nil {
		fetch := min(v.Size, remoteTailCap)
		var err error
		if data, err = r.host.Run(fmt.Sprintf("if %s; then head -c %d %s | tail -c %d; else exit 1; fi",
			remoteSessionFile(path), v.Size, quoted, fetch), 60*time.Second); err != nil {
			return nil, err
		}
	}
	if len(data) > remoteTailCap {
		data = append([]byte(nil), data[len(data)-remoteTailCap:]...)
	}

	r.mu.Lock()
	keepTail(tails, path, &cachedTail{version: v, data: data, read: time.Now()}, budget)
	r.mu.Unlock()
	return data, nil
}

// keepTail caches a file's tail within budget bytes, the tails read longest ago going first: a file not cached before
// goes uncached rather than push out one read within recentlyRead. The caller holds r.mu.
func keepTail(tails map[string]*cachedTail, path string, tail *cachedTail, budget int) {
	_, wasCached := tails[path]
	tails[path] = tail
	size := 0
	for _, t := range tails {
		size += len(t.data)
	}
	for size > budget {
		oldest := ""
		for k, t := range tails {
			if k != path && (oldest == "" || t.read.Before(tails[oldest].read)) {
				oldest = k
			}
		}
		if oldest == "" {
			return
		}
		if time.Since(tails[oldest].read) < recentlyRead && !wasCached {
			delete(tails, path)
			return
		}
		size -= len(tails[oldest].data)
		delete(tails, oldest)
	}
}

// ResolveSession finds the session file of herdr's agent_session for an agent kind (claude, codex, gemini; "" tries
// every format): the exact id in the folder's own place first, else a search across the format's tree.
func (r *Reader) ResolveSession(agent, cwd, kind, value string) (string, error) {
	// the agent's own hooks chose these: a path only under one of the session roots, an id only what looks like one
	if kind == "path" {
		clean := rpath.For(r.host.Home()).Clean(value)
		if !(strings.HasSuffix(clean, ".jsonl") || strings.HasSuffix(clean, ".json")) || !underRoots(r.Roots(), rpath.For(r.host.Home()), clean) || !r.regularSession(clean) {
			return "", fmt.Errorf("session path %q is not a regular session file under one of the machine's session roots", value)
		}
		return clean, nil
	}
	if !text.SessionID(value) {
		return "", fmt.Errorf("session id %q does not look like one", value)
	}
	if path := r.activePath(value); path != "" {
		style := rpath.For(r.host.Home())
		clean := style.Clean(path)
		if underRoots(r.Roots(), style, clean) && (!r.host.Local() || sessionFile(clean)) {
			return clean, nil
		}
	}
	format := FormatOfAgent(agent)
	roots, rootVersion := r.rootsSnapshot()
	r.mu.Lock()
	key := sessionKey{format: format, cwd: cwd, id: value, rootVersion: rootVersion}
	cached, ok := r.sessionPath[key]
	cachedAt := r.sessionAt[key]
	missedAt, missed := r.sessionMiss[key]
	r.mu.Unlock()
	if ok {
		recheck := format == Codex || format == "" && FormatOf(cached) == Codex
		fresh := !recheck || time.Since(cachedAt) < codexResolveFresh
		if fresh && !r.host.Local() {
			return cached, nil
		}
		if fresh {
			if _, err := os.Stat(cached); err == nil {
				return cached, nil
			}
		}
	}
	missedFor := 30 * time.Second
	if format == Codex {
		missedFor = codexResolveFresh
	}
	if missed && time.Since(missedAt) < missedFor {
		return "", os.ErrNotExist
	}

	var path string
	miss := func(err error) (string, error) {
		if errors.Is(err, os.ErrNotExist) { // only a file known not there is remembered: a machine not answering is no miss
			r.mu.Lock()
			r.sessionMiss[key] = time.Now()
			r.mu.Unlock()
		}
		return "", err
	}
	if r.host.Local() {
		var err error
		if path, err = findSessionIn(roots, format, cwd, value); err != nil {
			return miss(err)
		}
	} else {
		out, err := shell.Text(r.host, findScript(roots, format, r.host.Home(), cwd, value), 15*time.Second)
		if err != nil {
			return miss(err)
		}
		if path = strings.TrimSpace(out); path == "" {
			return miss(os.ErrNotExist)
		}
		if strings.ContainsAny(path, "\r\n") {
			return miss(errors.New("the machine named more than one session file"))
		}
	}
	r.mu.Lock()
	r.sessionPath[key] = path
	r.sessionAt[key] = time.Now()
	delete(r.sessionMiss, key)
	r.mu.Unlock()
	return path, nil
}

// findScript is findSessionIn as a shell line for a remote machine: the formats tried in turn, the folders of each in
// turn, the first path printed.
func findScript(roots Roots, format Format, home, cwd, id string) string {
	formats := []Format{format}
	if format == "" {
		formats = []Format{Claude, Codex, Gemini}
	}
	style := rpath.For(home)
	var steps []string
	for _, f := range formats {
		switch f {
		case Codex:
			for _, root := range roots[Codex] {
				steps = append(steps, codexFindScript(root, id))
			}
		case Gemini:
			steps = append(steps, geminiFindScript(home, id))
		default:
			for _, projects := range roots[Claude] {
				project := style.Join(projects, EncodePath(cwd))
				quotedProject, quotedProjects := script.Quote(style.Slash(project)), script.Quote(style.Slash(projects))
				steps = append(steps, `if [ -d `+quotedProject+` ] && [ ! -L `+quotedProject+` ]; then find `+quotedProject+
					` -maxdepth 1 -type f -name `+script.Quote(id+".jsonl")+` 2>/dev/null | head -1; else find `+quotedProjects+
					` -maxdepth 2 -type f -name `+script.Quote(id+".jsonl")+` 2>/dev/null | head -1; fi`)
			}
		}
	}
	var b strings.Builder
	for i, step := range steps {
		b.WriteString(`p=$(` + step + `); [ -n "$p" ] && { printf %s "$p"; exit 0; }`)
		if i < len(steps)-1 {
			b.WriteString("; ")
		}
	}
	b.WriteString("; exit 0")
	return b.String()
}

// Live is the file a session's conversation goes on in now: the end of its chain of continuations.
// A compaction moves the conversation to a new file (Continuation) while herdr goes on naming the first.
func (r *Reader) Live(path string) string {
	chain := r.Chain(path)
	return chain[len(chain)-1]
}

// Chain is the file and every file it went on in, oldest first, each once. Only Claude Code moves a conversation
// between files: another format's chain is the file alone.
func (r *Reader) Chain(path string) []string {
	chain := []string{path}
	if FormatOf(path) != Claude {
		return chain
	}
	seen := map[string]bool{path: true}
	for next := r.nextContinuation(path); next != "" && !seen[next]; next = r.nextContinuation(next) {
		seen[next] = true
		chain = append(chain, next)
	}
	return chain
}

// Lineage is every file a conversation has lived in, oldest first: the files path continues, path, and the files it went on in.
func (r *Reader) Lineage(path string) []string {
	chain := r.Chain(path)
	seen := map[string]bool{}
	for _, p := range chain {
		seen[p] = true
	}
	for earlier := r.Predecessor(chain[0]); earlier != "" && !seen[earlier]; earlier = r.Predecessor(earlier) {
		seen[earlier] = true
		chain = append([]string{earlier}, chain...)
	}
	return chain
}

// Predecessor is the file a session file continues after a compaction ("" when none), cached for good.
func (r *Reader) Predecessor(path string) string {
	if FormatOf(path) != Claude {
		return ""
	}
	r.mu.Lock()
	earlier, ok := r.predecessor[path]
	r.mu.Unlock()
	if ok {
		return earlier
	}
	if r.host.Local() {
		earlier = Predecessor(path)
	} else if out, err := shell.Text(r.host, PredecessorScript(script.Quote(path)), 30*time.Second); err == nil {
		earlier = strings.TrimSpace(out)
	} else {
		return "" // not cached: asked again next time
	}
	r.mu.Lock()
	r.predecessor[path] = earlier
	r.mu.Unlock()
	return earlier
}

// nextContinuation is one hop, cached for a minute; a remote that does not answer keeps what was known for another
// minute, and when nothing was, is asked again next time.
func (r *Reader) nextContinuation(path string) string {
	r.mu.Lock()
	known, ok := r.continuation[path]
	r.mu.Unlock()
	if ok && time.Since(known.at) <= time.Minute {
		return known.next
	}
	var next string
	if r.host.Local() {
		next = Continuation(path)
	} else if out, err := shell.Text(r.host, ContinuationScript(script.Quote(path)), 20*time.Second); err == nil {
		next = strings.TrimSpace(out)
	} else if ok {
		next = known.next
	} else {
		return ""
	}
	r.mu.Lock()
	r.continuation[path] = continuation{next: next, at: time.Now()}
	r.mu.Unlock()
	return next
}
