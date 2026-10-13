package transcript

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	tails        map[string]*cachedTail  // remote files by path
	sessionPath  map[string]string       // session id -> resolved path
	sessionMiss  map[string]time.Time    // session id -> when it was last not found
	continuation map[string]continuation // session file -> its continuation, as last looked
	predecessor  map[string]string       // session file -> the file it continues ("" when none)
}

type cachedTail struct {
	version Version
	data    []byte
}

type continuation struct {
	next string
	at   time.Time
}

const remoteTailCap = 8 * 1024 * 1024

// NewReader reads the transcripts on host.
func NewReader(host shell.Host) *Reader {
	return &Reader{host: host, tails: map[string]*cachedTail{}, sessionPath: map[string]string{}, sessionMiss: map[string]time.Time{},
		continuation: map[string]continuation{}, predecessor: map[string]string{}}
}

// Version stats the session file; the version doubles as the ETag.
func (r *Reader) Version(path string) (Version, error) {
	if r.host.Local() {
		return Stat(path)
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
	out := map[string]Version{}
	if r.host.Local() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return out, nil
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			if info, err := entry.Info(); err == nil {
				out[filepath.Join(dir, entry.Name())] = Version{Size: info.Size(), MTime: info.ModTime().UnixNano()}
			}
		}
		return out, nil
	}
	quoted := script.Quote(dir)
	listing := `[ -d ` + quoted + ` ] || exit 0; for f in ` + quoted + `/*.jsonl; do [ -f "$f" ] || continue; ` + script.Stat(`"$f"`, "%s %Y %n", "%z %m %N") + `; done`
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
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, err
		}
		return io.ReadAll(io.LimitReader(f, 256<<20))
	}
	return r.host.Run(fmt.Sprintf("tail -c +%d %s", offset+1, script.Quote(path)), 120*time.Second)
}

// Turns parses the session file at the given version, remotely from a cached tail grown by delta.
func (r *Reader) Turns(path string, v Version) ([]Turn, error) {
	if r.host.Local() {
		return Parse(path)
	}
	data, err := r.remoteTail(path, v)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// remoteTail returns the file tail, fetching only the delta since the cached copy when the file merely grew.
func (r *Reader) remoteTail(path string, v Version) ([]byte, error) {
	r.mu.Lock()
	cached := r.tails[path]
	r.mu.Unlock()
	if cached != nil && cached.version == v {
		return cached.data, nil
	}

	// every read is cut at the size the stat saw: what the agent wrote since belongs to the next version
	quoted := script.Quote(path)
	var data []byte
	if cached != nil && v.Size > cached.version.Size {
		// grew: fetch the delta only (tail -c +N is 1-indexed)
		delta, err := r.host.Run(fmt.Sprintf("tail -c +%d %s | head -c %d", cached.version.Size+1, quoted, v.Size-cached.version.Size), 30*time.Second)
		if err == nil {
			data = append(cached.data[:len(cached.data):len(cached.data)], delta...) // the cached slice is shared: copy
		}
	}
	if data == nil {
		fetch := min(v.Size, remoteTailCap)
		var err error
		if data, err = r.host.Run(fmt.Sprintf("head -c %d %s | tail -c %d", v.Size, quoted, fetch), 60*time.Second); err != nil {
			return nil, err
		}
	}
	if len(data) > remoteTailCap {
		data = append([]byte(nil), data[len(data)-remoteTailCap:]...)
	}

	r.mu.Lock()
	r.tails[path] = &cachedTail{version: v, data: data}
	if len(r.tails) > 8 {
		for k := range r.tails {
			if k != path {
				delete(r.tails, k)
				break
			}
		}
	}
	r.mu.Unlock()
	return data, nil
}

// ResolveSession finds the session file of herdr's agent_session: the exact id first, else a glob across project dirs.
func (r *Reader) ResolveSession(cwd, kind, value string) (string, error) {
	// the agent's own hooks chose these: a path only under ~/.claude/projects, an id only what looks like one
	if kind == "path" {
		clean := filepath.Clean(value)
		under := filepath.Join(r.host.Home(), ".claude", "projects") + "/"
		if !strings.HasSuffix(clean, ".jsonl") || !strings.HasPrefix(clean, under) {
			return "", fmt.Errorf("session path %q is not a session file under ~/.claude/projects", value)
		}
		return clean, nil
	}
	if !text.SessionID(value) {
		return "", fmt.Errorf("session id %q does not look like one", value)
	}
	r.mu.Lock()
	cached, ok := r.sessionPath[value]
	missedAt, missed := r.sessionMiss[value]
	r.mu.Unlock()
	if ok {
		if !r.host.Local() {
			return cached, nil
		}
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}
	if missed && time.Since(missedAt) < 30*time.Second {
		return "", os.ErrNotExist
	}

	var path string
	miss := func(err error) (string, error) {
		r.mu.Lock()
		r.sessionMiss[value] = time.Now()
		r.mu.Unlock()
		return "", err
	}
	if r.host.Local() {
		var err error
		if path, err = FindSessionByID(cwd, value); err != nil {
			return miss(err)
		}
	} else {
		projects := r.host.Home() + "/.claude/projects"
		primary := projects + "/" + EncodePath(cwd) + "/" + value + ".jsonl"
		find := `if [ -f ` + script.Quote(primary) + ` ]; then printf %s ` + script.Quote(primary) + `; else ls ` +
			script.Quote(projects) + `/*/` + script.Quote(value+".jsonl") + ` 2>/dev/null | head -1; fi`
		out, err := shell.Text(r.host, find, 15*time.Second)
		if err != nil {
			return miss(err)
		}
		if path = strings.TrimSpace(out); path == "" {
			return miss(os.ErrNotExist)
		}
	}
	r.mu.Lock()
	r.sessionPath[value] = path
	delete(r.sessionMiss, value)
	r.mu.Unlock()
	return path, nil
}

// Live is the file a session's conversation goes on in now: the end of its chain of continuations.
// A compaction moves the conversation to a new file (Continuation) while herdr goes on naming the first.
func (r *Reader) Live(path string) string {
	chain := r.Chain(path)
	return chain[len(chain)-1]
}

// Chain is the file and every file it went on in, oldest first, up to 8 hops.
func (r *Reader) Chain(path string) []string {
	chain := []string{path}
	for hops := 0; hops < 8; hops++ {
		next := r.nextContinuation(path)
		if next == "" {
			break
		}
		chain = append(chain, next)
		path = next
	}
	return chain
}

// Lineage is every file a conversation has lived in, oldest first: the files path continues, path, and the files it went on in.
func (r *Reader) Lineage(path string) []string {
	chain := r.Chain(path)
	for hops := 0; hops < 8; hops++ {
		earlier := r.Predecessor(chain[0])
		if earlier == "" {
			break
		}
		chain = append([]string{earlier}, chain...)
	}
	return chain
}

// Predecessor is the file a session file continues after a compaction ("" when none), cached for good.
func (r *Reader) Predecessor(path string) string {
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

// nextContinuation is one hop, cached for a minute.
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
	}
	r.mu.Lock()
	r.continuation[path] = continuation{next: next, at: time.Now()}
	r.mu.Unlock()
	return next
}
