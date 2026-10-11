package usage

import (
	"bytes"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/sshx"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/version"
)

// Display is a pane's meter for the roster: the model, how full its
// context is, what the session has cost so far.
type Display struct {
	Model   string  `json:"model"`
	Name    string  `json:"name"`              // the model's display name
	Context int64   `json:"context"`           // tokens the last response read
	Window  int     `json:"window"`            // the model's context window
	Cost    float64 `json:"cost"`              // USD, this file and the ones it continues
	Turns   int     `json:"turns"`             // API responses
	Tokens  Tokens  `json:"tokens"`            // the sums behind the cost
	Updated int64   `json:"updated,omitempty"` // unix ms of the last response
	Unknown bool    `json:"unknown,omitempty"` // a model with no price here
	Partial bool    `json:"partial,omitempty"` // only a tail was read: the cost is a floor
}

// Tracker keeps a running sum per transcript file, reading only what was
// appended since the last look — locally from the file, remotely as a
// `tail -c +N` over ssh — and at most every refreshEvery per file.
type Tracker struct {
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
	checked time.Time
	partial bool // only the tail was read: the cost is a floor, the context exact
}

const (
	refreshEvery = 20 * time.Second
	readCap      = 64 << 20 // more than this at once is not read: the tail is, and the sum says so
)

func NewTracker() *Tracker {
	return &Tracker{files: map[string]*fileState{}, panes: map[string]string{}, pred: map[string]string{}, subs: map[string][]string{}, subsChecked: map[string]time.Time{}}
}

func key(machineID, s string) string { return machineID + "|" + s }

// Observe keeps a pane's files current: the chain is oldest first, the
// last one live; each earlier one — compacted away — is read once and its
// sum counts toward the pane too.
func (t *Tracker) Observe(mc *machines.Machine, paneID string, chain []string) {
	if len(chain) == 0 {
		return
	}
	live := chain[len(chain)-1]
	t.mu.Lock()
	t.panes[key(mc.Cfg.ID, paneID)] = live
	for i := 1; i < len(chain); i++ {
		t.pred[key(mc.Cfg.ID, chain[i])] = chain[i-1]
	}
	t.mu.Unlock()
	t.refresh(mc, live, false)
	for _, p := range chain[:len(chain)-1] {
		t.refresh(mc, p, true)
	}
	// the session's subagents (<session>/subagents/*.jsonl) spend too: what
	// Claude Code's own counter shows includes them
	t.observeSubagents(mc, live)
}

func (t *Tracker) observeSubagents(mc *machines.Machine, live string) {
	k := key(mc.Cfg.ID, live)
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
	vers, err := mc.DirVersions(dir)
	if err != nil {
		return
	}
	files := make([]string, 0, len(vers))
	for p, v := range vers {
		files = append(files, p)
		t.read(mc, p, v)
	}
	sort.Strings(files)
	t.mu.Lock()
	t.subs[k] = files
	t.mu.Unlock()
}

func (t *Tracker) refresh(mc *machines.Machine, path string, once bool) {
	k := key(mc.Cfg.ID, path)
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

	v, err := mc.TranscriptVersion(path)
	if err != nil {
		return
	}
	t.read(mc, path, v)
}

// read brings a file's sum up to version v, reading only what was added
// since the last look.
func (t *Tracker) read(mc *machines.Machine, path string, v transcript.Version) {
	k := key(mc.Cfg.ID, path)
	t.mu.Lock()
	st := t.files[k]
	if st == nil {
		st = &fileState{}
		t.files[k] = st
	}
	have := st.version
	offset := st.offset
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
	data, err := mc.ReadFrom(path, offset)
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
	// parsed outside the lock: a big first read must not hold the roster
	var recs []transcript.UsageRecord
	_ = transcript.Lines(bytes.NewReader(data), 64*1024*1024, func(line []byte) {
		if !bytes.Contains(line, []byte(`"usage"`)) {
			return
		}
		if r, ok := transcript.UsageOf(line); ok { // sidechains spend too
			recs = append(recs, r)
		}
	})
	t.mu.Lock()
	defer t.mu.Unlock()
	if reset {
		st.acc = Accumulator{}
		st.partial = partial
	}
	for _, r := range recs {
		st.acc.Add(r)
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
	// the sum is read with the pending response counted
	sum, last := st.acc.Sum()
	if sum.Turns == 0 {
		return Display{}, false
	}
	d := Display{Model: last.Model, Cost: sum.Cost, Turns: sum.Turns, Tokens: sum.Tokens, Updated: last.At.UnixMilli(), Partial: st.partial}
	d.Context = tokensOf(last).Context()
	if m, ok := Lookup(last.Model); ok {
		d.Model, d.Name, d.Window = m.ID, m.Name, m.Window
	} else {
		d.Unknown = true
	}
	// the files this one continues, and the subagents of the live one
	more := t.subs[key(machineID, path)]
	for p, n := t.pred[key(machineID, path)], 0; p != "" && n < 8; p, n = t.pred[key(machineID, p)], n+1 {
		more = append(more, p)
	}
	for _, p := range more {
		if ps := t.files[key(machineID, p)]; ps != nil {
			a, _ := ps.acc.Sum()
			d.Cost += a.Cost
			d.Turns += a.Turns
			d.Tokens.Add(a.Tokens)
			d.Partial = d.Partial || ps.partial
		}
	}
	return d, true
}

// Script runs the report on a remote machine through its own remuda.
func Script(home string, since time.Time, tz string) string {
	q := sshx.Quote
	bin, guard := sshx.VersionedBin(home, version.Build)
	tzFlag := ""
	if tz != "" {
		tzFlag = " -tz " + q(tz)
	}
	return "if " + guard + "; then " + bin + " usage -since " + q(since.UTC().Format(time.RFC3339)) + tzFlag +
		"; else echo 'no remuda of this version on the machine (make deploy)' >&2; exit 3; fi"
}
