package transcript

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// A session's subagents: the agents a conversation spawned through its Agent tool, each a transcript of its own
// under <session>/subagents/ with a meta file beside it. They are read as the parent file is — in process here, by
// delta over ssh on a remote — and summed up once per version of each file, so a poll costs one stat round trip.

// Subagent is one agent a conversation spawned, as the chat shows it under the call that started it.
type Subagent struct {
	ID          string  `json:"id"`      // the file's stem: agent-<hex>
	ToolID      string  `json:"tool_id"` // the Agent tool_use that spawned it
	Description string  `json:"description"`
	Kind        string  `json:"kind,omitempty"`   // fork, Explore, general-purpose, a custom agent…
	Model       string  `json:"model,omitempty"`  // "" when it inherits the parent's
	Branch      string  `json:"branch,omitempty"` // the branch of a worktree of its own
	Status      string  `json:"status"`           // working | quiet | done
	Tools       int     `json:"tools"`            // its tool calls so far
	Failed      int     `json:"failed,omitempty"` // of them, the failed ones
	Doing       *Part   `json:"doing,omitempty"`  // the tool call still without a result
	Report      string  `json:"report,omitempty"` // the start of the text its last turn ended with
	Started     string  `json:"started,omitempty"`
	Ended       string  `json:"ended,omitempty"` // its last record
	Took        float64 `json:"took,omitempty"`  // seconds from its first record to its last
}

// The three states a subagent is in: at work, silent for a while without having finished, finished.
const (
	SubagentWorking = "working"
	SubagentQuiet   = "quiet"
	SubagentDone    = "done"
)

// QuietAfter is how long an unfinished subagent may write nothing before it reads as quiet.
const QuietAfter = 10 * time.Minute

// ReportedAfter is how long after its last answer a subagent that wrote nothing more counts as done: the meta's
// finish is not written for every kind of agent, and a report is the last thing one writes.
const ReportedAfter = 20 * time.Second

const reportMax = 2048

// the meta file Claude Code writes beside a subagent's transcript
type subagentMeta struct {
	AgentType      string `json:"agentType"`
	Description    string `json:"description"`
	ToolUseID      string `json:"toolUseId"`
	Model          string `json:"model"`
	WorktreeBranch string `json:"worktreeBranch"`
	Finished       bool   `json:"finished"`
}

// cachedMeta is a meta file as read at a version of it.
type cachedMeta struct {
	version Version
	meta    subagentMeta
}

type subagentEntry struct {
	version     Version // of the transcript
	metaVersion Version
	meta        subagentMeta
	summary     Subagent
	used        uint64 // the reader's use count when it was last asked for, so the least used goes first
}

// SubagentsKept is how many subagents' summaries a reader keeps: a few long sessions on one machine hold hundreds each,
// and an evicted summary costs its whole transcript again over ssh
const SubagentsKept = 4096

type dirListing struct {
	at       time.Time
	versions map[string]Version
}

// SubagentsDir is where a session file's subagents live.
func SubagentsDir(sessionPath string) string {
	return strings.TrimSuffix(sessionPath, ".jsonl") + "/subagents"
}

// SubagentID says whether a subagent id can name a file: the stem Claude Code gives one, nothing that leaves the folder.
func SubagentID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// Subagents lists a session's subagents with what each is at, oldest first, and a stamp of the folder's files that
// moves whenever one of them does, for the chat's ETag. The folder is stat'ed in one round trip; only a transcript or
// meta file that changed is read again. No folder is no subagents.
func (r *Reader) Subagents(sessionPath string, now time.Time) ([]Subagent, string, error) {
	if FormatOf(sessionPath) != Claude { // only Claude Code keeps a session's subagents beside it
		return nil, "", nil
	}
	dir := SubagentsDir(sessionPath)
	versions, err := r.subagentFiles(dir, now)
	if err != nil {
		return nil, "", err
	}
	if len(versions) == 0 {
		return nil, "", nil
	}
	var out []Subagent
	for path, version := range versions {
		if strings.HasSuffix(path, ".jsonl") {
			out = append(out, r.subagentAt(path, version, versions, now))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Started != out[j].Started {
			return out[i].Started < out[j].Started
		}
		return out[i].ID < out[j].ID
	})
	return out, stamp(versions), nil
}

// SubagentsFor is the subagents a chat has a use for: the ones the calls given spawned — the Agent calls of the turns
// on screen, by their tool ids — and the ones whose transcript moved within QuietAfter of now, which may still be at
// work. A long session spawns hundreds, a gigabyte of transcripts between them: read whole for every opening of the
// chat, over ssh, they never arrived. The folder is listed in one round trip and its meta files, which say which call
// spawned which, read in one more; only the transcripts of the ones answered are read.
func (r *Reader) SubagentsFor(sessionPath string, spawnedBy map[string]bool, now time.Time) ([]Subagent, error) {
	if FormatOf(sessionPath) != Claude {
		return nil, nil
	}
	dir := SubagentsDir(sessionPath)
	versions, err := r.subagentFiles(dir, now)
	if err != nil || len(versions) == 0 {
		return nil, err
	}
	metas := r.metasOf(dir, versions)
	var out []Subagent
	for path, version := range versions {
		if !strings.HasSuffix(path, ".jsonl") {
			continue
		}
		if spawnedBy[metas[metaPathOf(path)].ToolUseID] || now.Sub(version.Time()) <= QuietAfter {
			out = append(out, r.subagentAt(path, version, versions, now))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Started != out[j].Started {
			return out[i].Started < out[j].Started
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// HasSubagent says the session's folder holds a subagent of that id.
func (r *Reader) HasSubagent(sessionPath, id string, now time.Time) bool {
	if !SubagentID(id) || FormatOf(sessionPath) != Claude {
		return false
	}
	dir := SubagentsDir(sessionPath)
	versions, err := r.subagentFiles(dir, now)
	if err != nil {
		return false
	}
	_, ok := versions[filepath.Join(dir, id+".jsonl")]
	return ok
}

// metaPathOf is the meta file beside a subagent's transcript.
func metaPathOf(transcriptPath string) string {
	return strings.TrimSuffix(transcriptPath, ".jsonl") + ".meta.json"
}

// metasKept is how many meta files a reader remembers; past it the memory starts over.
const metasKept = 16384

// metasOf is the meta of every subagent a folder's listing names, by the meta file's path. The ones this reader has
// not read at the version listed are read: each file here, the whole folder in one round trip on a remote.
func (r *Reader) metasOf(dir string, versions map[string]Version) map[string]subagentMeta {
	out := map[string]subagentMeta{}
	var missing []string
	r.mu.Lock()
	for path, version := range versions {
		if !strings.HasSuffix(path, ".meta.json") {
			continue
		}
		if known, ok := r.metas[path]; ok && known.version == version {
			out[path] = known.meta
			continue
		}
		missing = append(missing, path)
	}
	r.mu.Unlock()
	if len(missing) == 0 {
		return out
	}
	read := map[string]subagentMeta{}
	if r.host.Local() {
		for _, path := range missing {
			raw, err := ReadSession(path)
			var meta subagentMeta
			if err == nil && json.Unmarshal(raw, &meta) == nil {
				read[path] = meta
			}
		}
	} else if answer, err := shell.Text(r.host, metasScript(dir), 20*time.Second); err == nil {
		for name, meta := range parseMetas(answer) {
			read[filepath.Join(dir, name)] = meta
		}
	}
	r.mu.Lock()
	if len(r.metas)+len(read) > metasKept {
		r.metas = map[string]cachedMeta{}
	}
	for _, path := range missing {
		if meta, ok := read[path]; ok {
			r.metas[path] = cachedMeta{version: versions[path], meta: meta}
			out[path] = meta
		}
	}
	r.mu.Unlock()
	return out
}

// metasScript prints every meta file of a subagents folder, each after its name: a few hundred bytes a subagent,
// four megabytes at most.
func metasScript(dir string) string {
	return "cd " + script.Quote(dir) + ` 2>/dev/null || exit 0
for f in *.meta.json; do
  [ -f "$f" ] || continue
  printf '\036%s\037' "$f"
  cat "$f"
done | head -c 4194304`
}

// parseMetas reads metasScript's answer: the metas by file name; one that does not parse, the last one cut by the
// cap among them, is left out.
func parseMetas(answer string) map[string]subagentMeta {
	out := map[string]subagentMeta{}
	for _, piece := range strings.Split(answer, script.RS) {
		name, raw, ok := strings.Cut(piece, script.US)
		if !ok || !strings.HasSuffix(name, ".meta.json") || strings.ContainsAny(name, "/\\") {
			continue
		}
		var meta subagentMeta
		if json.Unmarshal([]byte(raw), &meta) == nil {
			out[name] = meta
		}
	}
	return out
}

// RecentSubagents is a session's subagents whose transcript moved within QuietAfter of now, each with what it is at:
// the only ones that can be at work, since one silent for longer reads quiet or done. Only their files are read, the
// folder listed in one round trip, so the hundreds a long session spawned and left cost nothing more, here or over ssh.
func (r *Reader) RecentSubagents(sessionPath string, now time.Time) ([]Subagent, error) {
	if FormatOf(sessionPath) != Claude {
		return nil, nil
	}
	versions, err := r.subagentFiles(SubagentsDir(sessionPath), now)
	if err != nil {
		return nil, err
	}
	var out []Subagent
	for path, version := range versions {
		if strings.HasSuffix(path, ".jsonl") && now.Sub(version.Time()) <= QuietAfter {
			out = append(out, r.subagentAt(path, version, versions, now))
		}
	}
	return out, nil
}

// subagentAt is one subagent's summary as its files stand in the folder's listing, settled at now.
func (r *Reader) subagentAt(path string, version Version, versions map[string]Version, now time.Time) Subagent {
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	metaPath := strings.TrimSuffix(path, ".jsonl") + ".meta.json"
	entry := r.subagentEntry(path, id, version, metaPath, versions[metaPath])
	summary := entry.summary
	summary.settle(entry.meta.Finished, entry.summary.Report != "" && entry.summary.Doing == nil, version.Time(), now)
	return summary
}

// SubagentTurns is one subagent's conversation, for the viewer: its own records kept (they are all sidechain ones).
func (r *Reader) SubagentTurns(sessionPath, id string) ([]Turn, Version, error) {
	if !SubagentID(id) {
		return nil, Version{}, fmt.Errorf("subagent id %q does not look like one", id)
	}
	path := filepath.Join(SubagentsDir(sessionPath), id+".jsonl")
	version, err := r.Version(path)
	if err != nil {
		return nil, Version{}, err
	}
	turns, err := r.subagentFile(path, version)
	return turns, version, err
}

// subagentEntry is the summary of one subagent at the versions given, from the cache when both files stand as read.
func (r *Reader) subagentEntry(path, id string, version Version, metaPath string, metaVersion Version) *subagentEntry {
	r.mu.Lock()
	entry := r.subagents[path]
	if entry != nil {
		r.subagentUses++
		entry.used = r.subagentUses
	}
	r.mu.Unlock()
	if entry != nil && entry.version == version && entry.metaVersion == metaVersion {
		return entry
	}
	fresh := &subagentEntry{version: version, metaVersion: metaVersion}
	if entry != nil {
		fresh.meta = entry.meta
	}
	if entry == nil || entry.metaVersion != metaVersion {
		r.mu.Lock()
		known, read := r.metas[metaPath]
		r.mu.Unlock()
		if read && known.version == metaVersion {
			fresh.meta = known.meta
		} else if raw, err := r.ReadFrom(metaPath, 0); err == nil {
			var meta subagentMeta
			if json.Unmarshal(raw, &meta) == nil {
				fresh.meta = meta
			}
		}
	}
	if entry != nil && entry.version == version {
		fresh.summary = entry.summary
	} else {
		turns, _ := r.subagentFile(path, version)
		fresh.summary = summarize(id, turns)
	}
	fresh.summary.ToolID, fresh.summary.Description, fresh.summary.Kind, fresh.summary.Branch = fresh.meta.ToolUseID, fresh.meta.Description, fresh.meta.AgentType, fresh.meta.WorktreeBranch
	if fresh.meta.Model != "inherit" {
		fresh.summary.Model = fresh.meta.Model
	}
	r.mu.Lock()
	if _, kept := r.subagents[path]; !kept && len(r.subagents) >= SubagentsKept {
		r.forgetLeastUsedSubagent()
	}
	r.subagentUses++
	fresh.used = r.subagentUses
	r.subagents[path] = fresh
	r.mu.Unlock()
	return fresh
}

// forgetLeastUsedSubagent drops the summary asked for longest ago; the lock is held.
func (r *Reader) forgetLeastUsedSubagent() {
	oldest, at := "", uint64(0)
	for path, entry := range r.subagents {
		if oldest == "" || entry.used < at {
			oldest, at = path, entry.used
		}
	}
	delete(r.subagents, oldest)
}

// subagentFile parses a subagent's transcript at a version: in process here, from the delta-grown tail on a remote.
func (r *Reader) subagentFile(path string, version Version) ([]Turn, error) {
	if r.host.Local() {
		return parseFile(path, true)
	}
	data, err := r.remoteTailIn(r.subTails, path, version, remoteSubTailsBudget)
	if err != nil {
		return nil, err
	}
	return parseBytes(data, true)
}

// subagentFiles is the versions of a session's subagent transcripts and meta files; on a remote the listing is kept
// a few seconds, since the chat asks every two.
func (r *Reader) subagentFiles(dir string, now time.Time) (map[string]Version, error) {
	if r.host.Local() {
		return r.dirVersions(dir, ".jsonl", ".meta.json")
	}
	r.mu.Lock()
	known, ok := r.subDirs[dir]
	r.mu.Unlock()
	if ok && now.Sub(known.at) < 5*time.Second {
		return known.versions, nil
	}
	versions, err := r.dirVersions(dir, ".jsonl", ".meta.json")
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if len(r.subDirs) >= 64 {
		r.subDirs = map[string]dirListing{}
	}
	r.subDirs[dir] = dirListing{at: now, versions: versions}
	r.mu.Unlock()
	return versions, nil
}

// summarize reads a subagent's turns into what the chat says of it: its calls, what it is on, what it answered, its span.
func summarize(id string, turns []Turn) Subagent {
	s := Subagent{ID: id}
	for _, turn := range turns {
		for _, part := range turn.Parts {
			if part.Type != "tool" {
				continue
			}
			s.Tools++
			if part.IsError {
				s.Failed++
			}
		}
	}
	if len(turns) == 0 {
		return s
	}
	s.Started = turns[0].Ts
	last := turns[len(turns)-1]
	s.Ended = last.ended
	if s.Ended == "" {
		s.Ended = last.Ts
	}
	s.Took = secondsBetween(s.Started, s.Ended)
	if last.Role != "assistant" {
		return s
	}
	if n := len(last.Parts); n > 0 {
		if part := last.Parts[n-1]; part.Type == "tool" && part.Output == "" && part.Took == 0 && !part.IsError {
			running := part
			s.Doing = &running
		}
	}
	if report := last.Answer(); report != "" {
		s.Report = cutRunes(report, reportMax)
	}
	return s
}

// settle is the status: the meta's finish, or an answer followed by silence for ReportedAfter, says done; a
// transcript silent past QuietAfter without one says quiet; else the agent is at work.
func (s *Subagent) settle(finished, answered bool, lastWrite, now time.Time) {
	switch {
	case finished || (answered && now.Sub(lastWrite) > ReportedAfter):
		s.Status = SubagentDone
		s.Doing = nil
	case now.Sub(lastWrite) > QuietAfter:
		s.Status = SubagentQuiet
	default:
		s.Status = SubagentWorking
	}
}

// SubagentsStamp is the folder's stamp alone, no file read: "" without a subagent.
func (r *Reader) SubagentsStamp(sessionPath string, now time.Time) string {
	if FormatOf(sessionPath) != Claude {
		return ""
	}
	versions, err := r.subagentFiles(SubagentsDir(sessionPath), now)
	if err != nil || len(versions) == 0 {
		return ""
	}
	return stamp(versions)
}

// stamp is a short hash of every listed file's name and version, in a fixed order.
func stamp(versions map[string]Version) string {
	names := make([]string, 0, len(versions))
	for name := range versions {
		names = append(names, name)
	}
	sort.Strings(names)
	h := fnv.New64a()
	for _, name := range names {
		fmt.Fprintf(h, "%s:%d:%d;", filepath.Base(name), versions[name].Size, versions[name].MTime)
	}
	return fmt.Sprintf("%x", h.Sum64())
}

func cutRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// parseFile parses a transcript on this machine, keeping or dropping the sidechain records.
func parseFile(path string, keepSidechain bool) ([]Turn, error) {
	f, err := OpenSession(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > tailWindow {
		if _, err := f.Seek(fi.Size()-tailWindow, 0); err != nil {
			return nil, err
		}
	}
	return parseRecords(f, keepSidechain)
}
