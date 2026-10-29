package restart

// The chats a reboot stops. remuda remembers every agent pane's kind, session
// and folder in <state>/chats.json; when a machine's herdr comes back with
// those panes as bare shells — the watcher's first snapshot after the machine
// was away, or after remuda's own start — they are listed as stopped, the
// page offers one Resume, and Recover starts each on its session. Nothing
// starts by itself.

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/transcript"
)

const chatsFile = "chats.json"

// stoppedGrace is how long a pane must stay a bare shell before it is listed as stopped: herdr brings the agents it
// started itself back within a second of restoring their panes, and those are not stopped at all.
var stoppedGrace = 8 * time.Second

// Chat is what the memory keeps of an agent pane: enough to bring it back.
type Chat struct {
	Kind    string `json:"kind"`
	Session string `json:"session"`
	Dir     string `json:"dir"`
	Title   string `json:"title,omitempty"`
}

// Stopped is a remembered chat whose pane came back as a bare shell; the roster carries it.
type Stopped struct {
	Pane  string `json:"pane"`
	Agent string `json:"agent"`
	Title string `json:"title"`
	chat  Chat
}

// Memory remembers the chats and the ones a reboot stopped.
type Memory struct {
	dir string
	// Note records what a recovery did, in the activity; nil logs only.
	Note func(machineID, title, body string)

	mu      sync.Mutex
	chats   map[string]Chat      // machine|pane
	shells  map[string]time.Time // machine|pane → since when a remembered pane has stood as a bare shell, unwatched
	stopped map[string][]Stopped // per machine, in pane order
	paused  map[string]bool      // machines whose herdr remuda restarts itself: the shells are its own to bring back
}

// OpenMemory reads the chats remembered in dir; "" keeps them in memory alone.
func OpenMemory(dir string) *Memory {
	m := &Memory{dir: dir, chats: map[string]Chat{}, shells: map[string]time.Time{}, stopped: map[string][]Stopped{}, paused: map[string]bool{}}
	if dir != "" {
		if err := state.LoadJSON(dir, chatsFile, &m.chats); err != nil {
			log.Printf("%s: %v — starting with no chat remembered", chatsFile, err)
		}
		if m.chats == nil {
			m.chats = map[string]Chat{}
		}
	}
	return m
}

// Observe folds a machine's snapshot in: an agent pane with a session is remembered; a remembered pane that is a
// bare shell in the same folder was stopped by a reboot when the change was not watched happening (the snapshot is
// the machine's first, or the first after its herdr was away) and has stood so for stoppedGrace, and is forgotten
// when the person left the agent themselves; a pane no longer there is forgotten too.
func (m *Memory) Observe(e events.Snapshot) {
	if e.Snap == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.paused[e.Machine] {
		return
	}
	prefix := e.Machine + "|"
	seen := map[string]bool{}
	var stopped []Stopped
	changed := false
	for i := range e.Snap.Panes {
		p := &e.Snap.Panes[i]
		key := prefix + p.PaneID
		seen[key] = true
		if _, known := agents[p.Agent]; known {
			delete(m.shells, key)
			if sid := sessionOf(p); sid != "" {
				c := Chat{Kind: p.Agent, Session: sid, Dir: paneDir(e.Snap, p), Title: firstNonEmpty(e.Snap.WorkspaceLabel(p.WorkspaceID), p.DisplayTitle())}
				if old, ok := m.chats[key]; !ok || old != c {
					m.chats[key] = c
					changed = true
				}
			}
			continue
		}
		c, ok := m.chats[key]
		if !ok {
			continue
		}
		since, unwatched := m.shells[key]
		if p.Agent == "" && p.Dir() == c.Dir && (unwatched || e.First) {
			if !unwatched {
				since = e.At
				m.shells[key] = since
			}
			if e.At.Sub(since) >= stoppedGrace {
				stopped = append(stopped, Stopped{Pane: p.PaneID, Agent: c.Kind, Title: firstNonEmpty(c.Title, p.DisplayTitle()), chat: c})
			}
			continue
		}
		delete(m.chats, key)
		delete(m.shells, key)
		changed = true
	}
	for key := range m.chats {
		if strings.HasPrefix(key, prefix) && !seen[key] {
			delete(m.chats, key)
			delete(m.shells, key)
			changed = true
		}
	}
	sort.Slice(stopped, func(i, j int) bool { return stopped[i].Pane < stopped[j].Pane })
	if len(stopped) == 0 {
		delete(m.stopped, e.Machine)
	} else {
		m.stopped[e.Machine] = stopped
	}
	if changed {
		m.saveLocked()
	}
}

// paneDir is the folder a chat lives in: the pane's own, else its workspace's.
func paneDir(snap *herdr.Snapshot, p *herdr.PaneInfo) string {
	if dir := p.Dir(); dir != "" {
		return dir
	}
	if ws := snap.Workspace(p.WorkspaceID); ws != nil {
		return snap.WorkspaceDir(ws)
	}
	return ""
}

func (m *Memory) saveLocked() {
	if m.dir == "" {
		return
	}
	if err := state.SaveJSON(m.dir, chatsFile, m.chats); err != nil {
		log.Printf("%s: %v", chatsFile, err)
	}
}

// Stopped lists the machine's chats a reboot stopped, in pane order.
func (m *Memory) Stopped(machineID string) []Stopped {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Stopped(nil), m.stopped[machineID]...)
}

// Forget drops everything remembered of a machine (it left the registry).
func (m *Memory) Forget(machineID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for key := range m.chats {
		if strings.HasPrefix(key, machineID+"|") {
			delete(m.chats, key)
			changed = true
		}
	}
	m.dropShellsLocked(machineID)
	if changed {
		m.saveLocked()
	}
}

// dropShellsLocked clears what a machine stood accused of: its stopped list and the shells on their way to it.
func (m *Memory) dropShellsLocked(machineID string) {
	delete(m.stopped, machineID)
	for key := range m.shells {
		if strings.HasPrefix(key, machineID+"|") {
			delete(m.shells, key)
		}
	}
}

// Pause keeps the machine's shells off the stopped list while remuda restarts its herdr itself.
func (m *Memory) Pause(machineID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused[machineID] = true
	m.dropShellsLocked(machineID)
}

// Unpause ends a Pause.
func (m *Memory) Unpause(machineID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.paused, machineID)
}

// settle takes a pane off the stopped list once a recovery dealt with it; one that will not come back is forgotten.
func (m *Memory) settle(machineID, paneID string, back bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []Stopped
	for _, s := range m.stopped[machineID] {
		if s.Pane != paneID {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		delete(m.stopped, machineID)
	} else {
		m.stopped[machineID] = kept
	}
	delete(m.shells, machineID+"|"+paneID)
	if !back {
		if _, ok := m.chats[machineID+"|"+paneID]; ok {
			delete(m.chats, machineID+"|"+paneID)
			m.saveLocked()
		}
	}
}

// Stopped lists the machine's chats a reboot stopped; none without a memory.
func (r *Restarter) Stopped(machineID string) []Stopped {
	if r.Memory == nil {
		return nil
	}
	return r.Memory.Stopped(machineID)
}

// Observe is the watcher's hook: every snapshot goes into the memory.
func (r *Restarter) Observe(e events.Snapshot) {
	if r.Memory != nil {
		r.Memory.Observe(e)
	}
}

// Recover starts every chat the reboot stopped on the machine, each on its session under its pane's lock. A pane
// that is not a bare shell in the same folder any more, or whose folder or session file is gone, is skipped with the
// reason; a start that fails leaves the pane at its shell and on the list, so Resume can be pressed again.
func (r *Restarter) Recover(mc *machines.Machine, say func(string)) error {
	stopped := r.Stopped(mc.ID())
	if len(stopped) == 0 {
		return fmt.Errorf("no chat stopped by a reboot on %s", mc.Label())
	}
	client, _, err := mc.Ready()
	if err != nil {
		return err
	}
	say(fmt.Sprintf("%s stopped with the reboot on %s", chatsWord(len(stopped)), mc.Label()))
	checks := func(s Stopped) error {
		if info, err := mc.ResolvePath("", s.chat.Dir); err != nil || info.Kind != "dir" {
			return fmt.Errorf("its folder %s is gone — nothing to resume into", s.chat.Dir)
		}
		if s.chat.Kind == "claude" && !sessionFileExists(mc, s.chat) {
			return fmt.Errorf("its session file is gone — nothing to resume")
		}
		return nil
	}
	out := r.resume(client, mc.ID(), stopped, checks, say)
	var lines []string
	for _, o := range out {
		lines = append(lines, strings.TrimSpace(o.line))
	}
	if r.Memory != nil && r.Memory.Note != nil {
		title := fmt.Sprintf("%s back after the reboot on %s", chatsWord(len(out.back())), mc.Label())
		r.Memory.Note(mc.ID(), title, strings.Join(lines, "\n"))
	}
	if failed := out.failed(); len(failed) > 0 {
		return fmt.Errorf("%s not back: %s", chatsWord(len(failed)), strings.Join(failed, ", "))
	}
	return nil
}

// sessionFileExists looks for the Claude Code session file where the agent writes it — a fresh stat of the folder's
// own file, then the reader's lookup across project folders (which remembers a miss for half a minute, so a file
// written after an early look still counts).
func sessionFileExists(mc *machines.Machine, c Chat) bool {
	own := mc.Home() + "/.claude/projects/" + transcript.EncodePath(c.Dir) + "/" + c.Session + ".jsonl"
	if info, err := mc.ResolvePath("", own); err == nil && info.Kind != "missing" && info.Kind != "dir" {
		return true
	}
	_, err := mc.Transcripts.ResolveSession(c.Dir, "id", c.Session)
	return err == nil
}

// starter is what resuming a chat needs of herdr's client.
type starter interface {
	SessionSnapshot() (*herdr.Snapshot, error)
	AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error
}

// outcome is what became of one stopped chat.
type outcome struct {
	title  string
	back   bool
	failed bool // the start failed: the pane is at its shell and still on the list
	line   string
}

type outcomes []outcome

func (os outcomes) back() (list []string) {
	for _, o := range os {
		if o.back {
			list = append(list, o.title)
		}
	}
	return
}

func (os outcomes) failed() (list []string) {
	for _, o := range os {
		if o.failed {
			list = append(list, o.title)
		}
	}
	return
}

// resume is the recovery over a client: a fresh look at each pane, the checks, the start on the session.
func (r *Restarter) resume(client starter, machineID string, stopped []Stopped, checks func(Stopped) error, say func(string)) outcomes {
	var out outcomes
	for _, s := range stopped {
		out = append(out, r.resumeOne(client, machineID, s, checks, say))
	}
	return out
}

func (r *Restarter) resumeOne(client starter, machineID string, s Stopped, checks func(Stopped) error, say func(string)) outcome {
	if r.Lock != nil {
		l := r.Lock(machineID, s.Pane)
		l.Lock()
		defer l.Unlock()
	}
	a := agents[s.chat.Kind]
	o := outcome{title: s.Title}
	settle := func(back bool, line string) outcome {
		o.back, o.line = back, line
		say(line)
		if r.Memory != nil {
			r.Memory.settle(machineID, s.Pane, back)
		}
		return o
	}
	fail := func(line string) outcome {
		o.failed, o.line = true, line
		say(line)
		return o
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		return fail(fmt.Sprintf("· %s (%s): herdr did not answer: %v", s.Title, s.Pane, err))
	}
	p, found := Lookup(snap, s.Pane)
	switch {
	case !found:
		return settle(false, fmt.Sprintf("· %s (%s): the pane is gone", s.Title, s.Pane))
	case p.Agent == s.chat.Kind:
		return settle(true, fmt.Sprintf("· %s (%s): %s is back already", s.Title, s.Pane, a.Title))
	case p.Agent != "":
		return settle(false, fmt.Sprintf("· %s (%s): runs %q now — left as it is", s.Title, s.Pane, p.Agent))
	case p.Dir() != s.chat.Dir:
		return settle(false, fmt.Sprintf("· %s (%s): the shell is in %s now, not %s — left as it is", s.Title, s.Pane, p.Dir(), s.chat.Dir))
	}
	if err := checks(s); err != nil {
		return settle(false, fmt.Sprintf("· %s (%s): %v", s.Title, s.Pane, err))
	}
	args := a.resume(s.chat.Session)
	say(fmt.Sprintf("· %s (%s): %s %s", s.Title, s.Pane, a.Kind, strings.Join(args, " ")))
	if err := startResumed(client, s.Pane, a.Kind, args); err != nil {
		return fail(fmt.Sprintf("  ✗ %s: %v — the pane is at its shell; run `%s` there", s.Title, err, Resume(a.Kind, s.chat.Session)))
	}
	return settle(true, fmt.Sprintf("  ✓ %s is back in %s, session %s resumed", a.Title, s.Title, short(s.chat.Session)))
}

// startResumed starts the agent on its session under a name of the pane's own, waiting out a shell still coming up.
func startResumed(client starter, paneID, kind string, args []string) error {
	deadline := time.Now().Add(12 * time.Second)
	for {
		err := client.AgentStartWith(paneID, kind, agentName(kind, paneID), args, 90*time.Second)
		if err == nil || !(herdr.IsCode(err, "agent_pane_busy") || herdr.IsCode(err, "agent_not_ready")) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func chatsWord(n int) string {
	if n == 1 {
		return "1 chat"
	}
	return fmt.Sprintf("%d chats", n)
}
