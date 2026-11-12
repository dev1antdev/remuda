package usage

import (
	"bytes"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/version"
)

// Display is a pane's meter for the roster: the model, how full its context is, what the session has cost.
type Display struct {
	Model   string  `json:"model"`
	Name    string  `json:"name"`
	Context int64   `json:"context"`           // tokens the last response read
	Window  int     `json:"window"`            // the model's context window
	Cost    float64 `json:"cost"`              // USD, this file and the ones it continues
	Turns   int     `json:"turns"`             // API responses
	Tokens  Tokens  `json:"tokens"`            // the sums behind the cost
	Updated int64   `json:"updated,omitempty"` // unix ms of the last response
	Unknown bool    `json:"unknown,omitempty"` // a model with no price here
	Partial bool    `json:"partial,omitempty"` // only a tail was read: the cost is a floor
	Speed   string  `json:"speed,omitempty"`   // what the last response ran at, as the transcript says: "fast" in fast mode
}

// Files is where a machine's transcript bytes come from: a stat, a folder's stats, a read from an offset.
type Files interface {
	Version(path string) (transcript.Version, error)
	DirVersions(dir string) (map[string]transcript.Version, error)
	ReadFrom(path string, offset int64) ([]byte, error)
}

// Source is a machine's transcripts as the meter reads them: the bytes, and which files a pane's conversation lives in.
type Source interface {
	Files
	ResolveSession(agent, cwd, kind, value string) (string, error)
	Lineage(path string) []string
}

// Tracker keeps a running sum per transcript file, reading only what was appended since the last look.
type Tracker struct {
	busy        sync.Map // machine id → a read of its snapshot is under way
	mu          sync.Mutex
	files       map[string]*fileState // machine|path
	panes       map[string]string     // machine|pane → path
	pred        map[string]string     // machine|path → the file it continues
	subs        map[string][]string   // machine|path → its subagent transcripts
	subsChecked map[string]time.Time  // machine|path → when the subagent folder was last listed
}

type fileState struct {
	version transcript.Version
	offset  int64
	acc     Accumulator
	scanner transcript.UsageScanner // the file's reader, carrying what a format's earlier lines said (Codex's model)
	checked time.Time
	partial bool            // only the tail was read: the cost is a floor, the context exact
	ids     map[string]bool // the message ids counted here: a continuation copies its predecessor's tail, so a lineage counts each once
}

const (
	refreshEvery = 20 * time.Second
	readCap      = 64 << 20 // more than this at once is not read: the tail is, and the sum says so
)

func NewTracker() *Tracker {
	return &Tracker{files: map[string]*fileState{}, panes: map[string]string{}, pred: map[string]string{}, subs: map[string][]string{}, subsChecked: map[string]time.Time{}}
}

func key(machineID, s string) string { return machineID + "|" + s }

// ObserveSnapshot keeps every agent pane's meter current, one read per machine at a time and off the poll's path.
func (t *Tracker) ObserveSnapshot(machineID string, source Source, snap *herdr.Snapshot) {
	if _, running := t.busy.LoadOrStore(machineID, true); running {
		return
	}
	panes := append([]herdr.PaneInfo(nil), snap.Panes...)
	go func() {
		defer t.busy.Delete(machineID)
		for i := range panes {
			pane := &panes[i]
			if pane.Agent == "" || pane.AgentSession == nil {
				continue
			}
			first, err := source.ResolveSession(pane.AgentSession.Agent, pane.Dir(), pane.AgentSession.Kind, pane.AgentSession.Value)
			if err != nil {
				continue
			}
			t.Observe(machineID, source, pane.PaneID, source.Lineage(first))
		}
	}()
}

// Observe keeps a pane's files current: the chain is oldest first, the last one live, each earlier one read once and
// before the live one, which leaves out the responses it copied from them.
func (t *Tracker) Observe(machineID string, files Files, paneID string, chain []string) {
	if len(chain) == 0 {
		return
	}
	live := chain[len(chain)-1]
	t.mu.Lock()
	t.panes[key(machineID, paneID)] = live
	for i := 1; i < len(chain); i++ {
		t.pred[key(machineID, chain[i])] = chain[i-1]
	}
	t.mu.Unlock()
	for _, p := range chain[:len(chain)-1] {
		t.refresh(machineID, files, p, true, chain)
	}
	t.refresh(machineID, files, live, false, chain)
	// Claude Code's own counter includes <session>/subagents/*.jsonl
	t.observeSubagents(machineID, files, chain)
}

// elsewhere is the message ids the other files of a lineage already count: a continuation file opens with a copy of
// its predecessor's tail since the last compaction, the same responses record for record.
func (t *Tracker) elsewhere(machineID string, chain []string, path string) map[string]bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	counted := map[string]bool{}
	for _, p := range chain {
		if p == path {
			continue
		}
		if st := t.files[key(machineID, p)]; st != nil {
			for id := range st.ids {
				counted[id] = true
			}
		}
	}
	return counted
}

// observeSubagents keeps the live file's subagents current; a fork's transcript opens with a copy of the response that
// spawned it, which the file that made it counts.
func (t *Tracker) observeSubagents(machineID string, files Files, chain []string) {
	live := chain[len(chain)-1]
	k := key(machineID, live)
	t.mu.Lock()
	last := t.subsChecked[k]
	t.mu.Unlock()
	if time.Since(last) < refreshEvery {
		return
	}
	t.mu.Lock()
	t.subsChecked[k] = time.Now()
	t.mu.Unlock()
	dir := strings.TrimSuffix(live, ".jsonl") + "/subagents"
	vers, err := files.DirVersions(dir)
	if err != nil {
		return
	}
	paths := make([]string, 0, len(vers))
	for p := range vers {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	counted := append(append([]string(nil), chain...), paths...)
	for _, p := range paths {
		t.read(machineID, files, p, vers[p], func() map[string]bool { return t.elsewhere(machineID, counted, p) })
	}
	t.mu.Lock()
	t.subs[k] = paths
	t.mu.Unlock()
}

func (t *Tracker) refresh(machineID string, files Files, path string, once bool, chain []string) {
	k := key(machineID, path)
	t.mu.Lock()
	st := t.files[k]
	if st == nil {
		st = &fileState{}
		t.files[k] = st
	}
	if (once && st.offset > 0) || time.Since(st.checked) < refreshEvery {
		t.mu.Unlock()
		return
	}
	st.checked = time.Now()
	t.mu.Unlock()

	v, err := files.Version(path)
	if err != nil {
		return
	}
	t.read(machineID, files, path, v, func() map[string]bool { return t.elsewhere(machineID, chain, path) })
}

// read brings a file's sum up to version v, reading only what was added since the last look; a response whose id is
// in skip's answer, asked once there is something to read, is another file's and left out.
func (t *Tracker) read(machineID string, files Files, path string, v transcript.Version, skipOf func() map[string]bool) {
	k := key(machineID, path)
	t.mu.Lock()
	st := t.files[k]
	if st == nil {
		st = &fileState{}
		t.files[k] = st
	}
	have := st.version
	offset := st.offset
	scanner := st.scanner
	t.mu.Unlock()
	if v == have {
		return
	}
	reset := false
	if v.Size < offset { // rewritten: start over
		offset, reset = 0, true
	}
	partial := false
	if v.Size-offset > readCap { // too much at once: the tail, from a line boundary
		offset, reset, partial = v.Size-readCap, true, true
	}
	data, err := files.ReadFrom(path, offset)
	if err != nil {
		return
	}
	if partial {
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			return
		}
		data, offset = data[nl+1:], offset+int64(nl+1)
	}
	// only whole lines; the rest waits for the next look
	cut := bytes.LastIndexByte(data, '\n')
	if cut < 0 {
		return
	}
	data = data[:cut+1]
	if reset || scanner == nil {
		scanner = transcript.NewUsageScanner(path)
	}
	// parsed outside the lock: a big first read must not hold the roster
	skip := skipOf()
	var recs []transcript.UsageRecord
	_ = transcript.Lines(bytes.NewReader(data), 64*1024*1024, func(line []byte) {
		if r, ok := scanner.Scan(line); ok && !skip[r.MessageID] { // sidechains spend too
			recs = append(recs, r)
		}
	})
	t.mu.Lock()
	defer t.mu.Unlock()
	if reset {
		st.acc = Accumulator{}
		st.partial = partial
		st.ids = nil
	}
	st.scanner = scanner
	for _, r := range recs {
		st.acc.Add(r)
		if r.MessageID != "" {
			if st.ids == nil {
				st.ids = map[string]bool{}
			}
			st.ids[r.MessageID] = true
		}
	}
	st.offset = offset + int64(len(data))
	st.version = v // a partial last line changes the file when it completes
}

// Display is a pane's meter, or false when nothing was read yet.
func (t *Tracker) Display(machineID, paneID string) (Display, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	path := t.panes[key(machineID, paneID)]
	if path == "" {
		return Display{}, false
	}
	st := t.files[key(machineID, path)]
	if st == nil {
		return Display{}, false
	}
	sum, last := st.acc.Sum()
	d := Display{Cost: sum.Cost, Turns: sum.Turns, Tokens: sum.Tokens, Partial: st.partial}
	var predecessors []string
	for p, n := t.pred[key(machineID, path)], 0; p != "" && n < 8; p, n = t.pred[key(machineID, p)], n+1 {
		predecessors = append(predecessors, p)
	}
	if sum.Turns == 0 { // a file just moved to holds only what it copied: its last response is its predecessor's
		for _, p := range predecessors {
			if ps := t.files[key(machineID, p)]; ps != nil {
				if a, l := ps.acc.Sum(); a.Turns > 0 {
					last = l
					break
				}
			}
		}
	}
	for _, p := range append(append([]string(nil), t.subs[key(machineID, path)]...), predecessors...) {
		if ps := t.files[key(machineID, p)]; ps != nil {
			a, _ := ps.acc.Sum()
			d.Cost += a.Cost
			d.Turns += a.Turns
			d.Tokens.Add(a.Tokens)
			d.Partial = d.Partial || ps.partial
		}
	}
	if d.Turns == 0 || last.At.IsZero() && last.MessageID == "" {
		return Display{}, false
	}
	d.Model, d.Updated, d.Context, d.Speed = last.Model, last.At.UnixMilli(), tokensOf(last).Context(), last.Speed
	if m, ok := Lookup(last.Model); ok {
		d.Model, d.Name, d.Window = m.ID, m.Name, m.Window
	} else { // a model with no price here: named as the file names it, its window when the file says it (Codex does)
		d.Unknown = true
		d.Name, d.Window = last.Model, last.Window
	}
	return d, true
}

// Script runs the report on a remote machine through its own remuda.
func Script(home string, since time.Time, tz string) string {
	q := script.Quote
	bin, guard := script.Remuda(home, version.Build)
	tzFlag := ""
	if tz != "" {
		tzFlag = " -tz " + q(tz)
	}
	return "if " + guard + "; then " + bin + " usage -since " + q(since.UTC().Format(time.RFC3339)) + tzFlag +
		"; else echo 'no remuda of this version on the machine (make deploy)' >&2; exit 3; fi"
}
