package restart

// The chats a reboot stops. remuda remembers every agent pane's kind, session
// and folder in <state>/chats.json; when a machine's herdr comes back with
// those panes as bare shells — the watcher's first snapshot after the machine
// was away, or after remuda's own start — they are listed as stopped, the
// page offers them in "Resume after restart", and Recover starts the ones picked on
// their sessions. Nothing starts by itself unless the person switched on
// "Resume automatically next time" (OnStopped, then AutoRecover). A workspace put
// to sleep (Asleep) is the sleep's to wake: its panes are neither stopped nor closed.
// On a fleet moved to remuda-core (Core) a chat remembered in herdr, whose pane the
// core does not have, is listed the same way From herdr — never resumed by itself,
// nor while herdr's server still answers there — and carries on in a tab of a
// workspace on its folder (carryOn).

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/events"
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
	// Core marks a chat remembered off remuda-core; one without it, on a fleet moved to the core, ran in herdr
	Core bool `json:"core,omitempty"`
	// Ran is the folder the agent itself ran in when it was not its pane's: remuda-core gives a lost shell back in the
	// folder of the program that last held the terminal.
	Ran string `json:"ran,omitempty"`
}

// in says whether a shell standing in dir is where the chat was: its pane's folder, or the agent's own.
func (c Chat) in(dir string) bool {
	return dir == c.Dir || c.Ran != "" && dir == c.Ran
}

// ranIn is the folder the pane's agent runs in when it is not the pane's own, else "".
func ranIn(p *coreapi.PaneInfo, dir string) string {
	if p.ForegroundCWD != dir {
		return p.ForegroundCWD
	}
	return ""
}

// FromHerdr is the From of an agent that ran in herdr before its machine moved to remuda-core: its pane is gone with
// herdr, and it carries on in a tab of a workspace on its folder.
const FromHerdr = "herdr"

// Stopped is a remembered chat whose pane came back as a bare shell, or one that ran in herdr before the move to
// remuda-core (From); the roster carries it, with the folder and the command that brings it back, for the dialog to show.
type Stopped struct {
	Pane    string `json:"pane"`
	Agent   string `json:"agent"`
	Title   string `json:"title"`
	Dir     string `json:"dir"`
	Command string `json:"command"`
	From    string `json:"from,omitempty"`
	chat    Chat
}

// stoppedOf is the roster's entry for a chat a reboot stopped in a pane.
func stoppedOf(paneID string, c Chat, title string) Stopped {
	return Stopped{Pane: paneID, Agent: c.Kind, Title: title, Dir: c.Dir, Command: Resume(c.Kind, c.Session), chat: c}
}

// Memory remembers the chats and the ones a reboot stopped.
type Memory struct {
	dir string
	// Note records what a recovery did, in the activity; nil logs only.
	Note func(machineID, title, body string)
	// Asleep names a pane of a workspace the person put to sleep: its agent left at their word and waking brings it
	// back, so it is neither stopped by a reboot nor closed.
	Asleep func(machineID, workspaceID, paneID string) bool
	// OnStopped hears a machine whose list of chats a reboot stopped has gained some, outside the memory's lock.
	OnStopped func(machineID string)
	// Core says the fleet is reached through remuda-core: a chat remembered in herdr is then one to carry on there.
	Core func() bool
	// Sleepers is, by pane, the agents of the machine's workspaces put to sleep in herdr (sleep.Store.SleptInHerdr),
	// listed from herdr with the rest once the fleet is on the core; Woken takes one off there once it is dealt with.
	Sleepers func(machineID string) map[string]Chat
	Woken    func(machineID, paneID string)

	mu      sync.Mutex
	chats   map[string]Chat      // machine|pane
	shells  map[string]time.Time // machine|pane → since when a remembered pane has stood as a bare shell, unwatched
	stopped map[string][]Stopped // per machine, in pane order
	faces   map[string]paneFace  // machine|pane → its names when last seen, for the closed list
	closed  map[string][]Closed  // per machine, newest first: the agents whose panes went away

	reopening map[string]bool // machine|closed id: a reopen under way
}

// OpenMemory reads the chats remembered in dir and the agents closed there; "" keeps them in memory alone.
func OpenMemory(dir string) *Memory {
	m := &Memory{dir: dir, chats: map[string]Chat{}, shells: map[string]time.Time{}, stopped: map[string][]Stopped{}, faces: map[string]paneFace{}, closed: map[string][]Closed{}}
	if dir != "" {
		if err := state.LoadJSON(dir, chatsFile, &m.chats); err != nil {
			log.Printf("%s: %v — starting with no chat remembered", chatsFile, err)
		}
		if m.chats == nil {
			m.chats = map[string]Chat{}
		}
		if err := state.LoadJSON(dir, closedFile, &m.closed); err != nil {
			log.Printf("%s: %v — starting with no closed agent listed", closedFile, err)
		}
		if m.closed == nil {
			m.closed = map[string][]Closed{}
		}
	}
	return m
}

// Observe folds a machine's snapshot in: an agent pane with a session is remembered; a remembered pane that is a
// bare shell in the same folder was stopped by a reboot when the change was not watched happening (the snapshot is
// the machine's first, or the first after its herdr was away, or remuda-core gave the pane a new shell in place of a
// lost one: Restored) and has stood so for stoppedGrace, and is forgotten
// when the person left the agent themselves; a pane no longer there was closed — listed as such, then forgotten.
func (m *Memory) Observe(e events.Snapshot) {
	if m.observe(e) && m.OnStopped != nil {
		m.OnStopped(e.Machine)
	}
}

// observe is Observe under the lock; true when the machine's stopped list gained a pane.
func (m *Memory) observe(e events.Snapshot) bool {
	if e.Snap == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	onCore := m.Core != nil && m.Core()
	prefix := e.Machine + "|"
	seen := map[string]bool{}
	running := map[string]bool{} // the sessions an agent of the snapshot is on
	var stopped []Stopped
	changed := false
	for i := range e.Snap.Panes {
		p := &e.Snap.Panes[i]
		key := prefix + p.PaneID
		seen[key] = true
		if m.Asleep != nil && m.Asleep(e.Machine, p.WorkspaceID, p.PaneID) {
			if _, ok := m.chats[key]; ok {
				delete(m.chats, key)
				changed = true
			}
			delete(m.shells, key)
			delete(m.faces, key)
			continue
		}
		if _, known := agents[p.Agent]; known {
			delete(m.shells, key)
			if sid := sessionOf(p); sid != "" {
				running[sid] = true
				m.faces[key] = faceOf(e.Snap, p)
				dir := paneDir(e.Snap, p)
				c := Chat{Kind: p.Agent, Session: sid, Dir: dir, Ran: ranIn(p, dir), Title: firstNonEmpty(e.Snap.WorkspaceLabel(p.WorkspaceID), p.DisplayTitle()), Core: onCore}
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
		if p.Agent == "" && (unwatched || e.First || p.Restored) {
			if !unwatched {
				since = e.At
				m.shells[key] = since
			}
			// a restored pane reads the server's folder as its own for a moment: the folder is judged after the grace
			switch {
			case e.At.Sub(since) < stoppedGrace:
			case c.in(p.Dir()):
				stopped = append(stopped, stoppedOf(p.PaneID, c, firstNonEmpty(c.Title, p.DisplayTitle())))
			default:
				delete(m.chats, key)
				delete(m.shells, key)
				changed = true
			}
			continue
		}
		delete(m.chats, key)
		delete(m.shells, key)
		changed = true
	}
	closed := false
	for key, c := range m.chats {
		if !strings.HasPrefix(key, prefix) || seen[key] {
			continue
		}
		paneID := strings.TrimPrefix(key, prefix)
		switch {
		case onCore && !c.Core && !running[c.Session]: // ran in herdr before the move: carried on through Resume after restart
			stopped = append(stopped, herdrStopped(paneID, c))
			continue
		case onCore && !c.Core: // an agent of the core is on its session again
		default:
			m.noteClosedLocked(e.Machine, key, paneID, c, e.At)
			closed = true
		}
		delete(m.chats, key)
		delete(m.shells, key)
		changed = true
	}
	if onCore && m.Sleepers != nil {
		listed := map[string]bool{}
		for _, s := range stopped {
			listed[s.Pane] = true
		}
		for paneID, c := range m.Sleepers(e.Machine) {
			switch {
			case running[c.Session]: // carried on already, by hand
				m.wokenLocked(e.Machine, paneID)
			case !listed[paneID]:
				stopped = append(stopped, herdrStopped(paneID, c))
			}
		}
	}
	for key := range m.faces {
		if strings.HasPrefix(key, prefix) && !seen[key] {
			delete(m.faces, key)
		}
	}
	if closed {
		m.saveClosedLocked()
	}
	sort.Slice(stopped, func(i, j int) bool { return stopped[i].Pane < stopped[j].Pane })
	joined := gained(m.stopped[e.Machine], stopped)
	if len(stopped) == 0 {
		delete(m.stopped, e.Machine)
	} else {
		m.stopped[e.Machine] = stopped
	}
	if changed {
		m.saveLocked()
	}
	return joined
}

// gained says whether the new list names a pane a reboot stopped that the old one did not; herdr's agents are no
// reboot's, and nothing resumes them by itself.
func gained(old, now []Stopped) bool {
	had := map[string]bool{}
	for _, s := range old {
		had[s.Pane] = true
	}
	for _, s := range now {
		if s.From == "" && !had[s.Pane] {
			return true
		}
	}
	return false
}

// wokenLocked tells the sleep a pane from herdr is dealt with: back, or never coming back.
func (m *Memory) wokenLocked(machineID, paneID string) {
	if m.Woken != nil {
		m.Woken(machineID, paneID)
	}
}

// herdrStopped is the roster's entry for an agent that ran in herdr's pane before the move to remuda-core.
func herdrStopped(paneID string, c Chat) Stopped {
	s := stoppedOf(paneID, c, c.Title)
	s.From = FromHerdr
	return s
}

// paneDir is the folder a chat lives in: the pane's own, else its workspace's.
func paneDir(snap *coreapi.Snapshot, p *coreapi.PaneInfo) string {
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
	for key := range m.faces {
		if strings.HasPrefix(key, machineID+"|") {
			delete(m.faces, key)
		}
	}
	m.dropShellsLocked(machineID)
	if changed {
		m.saveLocked()
	}
	if _, listed := m.closed[machineID]; listed {
		delete(m.closed, machineID)
		m.saveClosedLocked()
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

// settle takes a pane off the stopped list once a recovery dealt with it; one that will not come back is forgotten, and
// so is herdr's pane, whose agent carries on in a pane of the core.
func (m *Memory) settle(machineID, paneID string, back bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	forget := !back
	var kept []Stopped
	for _, s := range m.stopped[machineID] {
		if s.Pane != paneID {
			kept = append(kept, s)
		} else if s.From == FromHerdr {
			forget = true
			m.wokenLocked(machineID, paneID)
		}
	}
	if len(kept) == 0 {
		delete(m.stopped, machineID)
	} else {
		m.stopped[machineID] = kept
	}
	delete(m.shells, machineID+"|"+paneID)
	if forget {
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

// Picked is the machine's stopped chats in the panes named, every one when none is named.
func (r *Restarter) Picked(machineID string, panes []string) []Stopped {
	stopped := r.Stopped(machineID)
	if len(panes) == 0 {
		return stopped
	}
	want := map[string]bool{}
	for _, p := range panes {
		want[p] = true
	}
	var picked []Stopped
	for _, s := range stopped {
		if want[s.Pane] {
			picked = append(picked, s)
		}
	}
	return picked
}

// Recovering says whether a recovery runs on the machine now: the page then offers no Resume of its own.
func (r *Restarter) Recovering(machineID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recovering[machineID]
}

// Recover starts the chats the reboot stopped on the machine in the panes picked (every one for none), each on its
// session under its pane's lock. A pane that is not a bare shell in the same folder any more, or whose folder or
// session file is gone, is skipped with the reason; a start that fails leaves the pane at its shell and on the list,
// so Resume can be pressed again.
func (r *Restarter) Recover(mc *machines.Machine, panes []string, say func(string)) error {
	return r.recover(mc, panes, false, say)
}

// AutoRecover is Recover of every stopped chat, run by itself because the person switched on "Resume automatically
// next time"; the activity's row says so.
func (r *Restarter) AutoRecover(mc *machines.Machine, say func(string)) error {
	return r.recover(mc, nil, true, say)
}

func (r *Restarter) recover(mc *machines.Machine, panes []string, auto bool, say func(string)) error {
	stopped := r.Picked(mc.ID(), panes)
	if auto {
		stopped = Rebooted(stopped)
	}
	if len(stopped) == 0 {
		return fmt.Errorf("no chat stopped by a reboot on %s", mc.Label())
	}
	client, _, err := mc.Ready()
	if err != nil {
		return err
	}
	if !r.claimRecovery(mc.ID()) {
		return fmt.Errorf("the chats stopped on %s are being resumed already", mc.Label())
	}
	defer r.releaseRecovery(mc.ID())
	if err := r.MayCarryOn(mc, stopped); err != nil {
		return err
	}
	fromHerdr, rebooted := splitHerdr(stopped)
	if len(rebooted) > 0 {
		say(fmt.Sprintf("%s stopped with the reboot on %s", chatsWord(len(rebooted)), mc.Label()))
	}
	if len(fromHerdr) > 0 {
		say(fmt.Sprintf("%s from herdr to carry on on %s", chatsWord(len(fromHerdr)), mc.Label()))
	}
	checks := func(s Stopped, dir string) error { return CanResume(mc, s.chat.Kind, s.chat.Session, dir) }
	out := append(r.resume(client, mc.ID(), rebooted, checks, say), r.carryOn(client, mc.ID(), fromHerdr, checks, say)...)
	var lines []string
	for _, o := range out {
		lines = append(lines, strings.TrimSpace(o.line))
	}
	if r.Memory != nil && r.Memory.Note != nil {
		title := fmt.Sprintf("%s back after the reboot on %s", chatsWord(len(out.back())), mc.Label())
		switch {
		case auto:
			title = fmt.Sprintf("%s resumed by itself after the reboot on %s", chatsWord(len(out.back())), mc.Label())
		case len(fromHerdr) > 0:
			title = fmt.Sprintf("%s back on %s after the move from herdr", chatsWord(len(out.back())), mc.Label())
		}
		r.Memory.Note(mc.ID(), title, strings.Join(lines, "\n"))
	}
	if missing := out.notBack(); len(missing) > 0 {
		return fmt.Errorf("%s not back: %s", chatsWord(len(missing)), strings.Join(missing, ", "))
	}
	return nil
}

// Rebooted is the stopped chats a reboot stopped, herdr's agents left out: nothing resumes those by itself.
func Rebooted(stopped []Stopped) []Stopped {
	_, rebooted := splitHerdr(stopped)
	return rebooted
}

func splitHerdr(stopped []Stopped) (fromHerdr, rebooted []Stopped) {
	for _, s := range stopped {
		if s.From == FromHerdr {
			fromHerdr = append(fromHerdr, s)
		} else {
			rebooted = append(rebooted, s)
		}
	}
	return fromHerdr, rebooted
}

// HerdrRunning refuses to carry herdr's agents on while its server answers on the machine: an agent there may still be
// on the conversation, and a second one resumed on it would write the same session file.
type HerdrRunning struct{ Machine string }

func (e *HerdrRunning) Error() string {
	return "herdr's server still runs on " + e.Machine + ": its agents may still be on these conversations, and a second agent on one would write the same session. Stop herdr's server there first."
}

// MayCarryOn says why herdr's agents among the stopped cannot carry on now — its server still answers on the machine
// (*HerdrRunning), or nobody can tell — nil when they can, or none is herdr's.
func (r *Restarter) MayCarryOn(mc *machines.Machine, stopped []Stopped) error {
	if fromHerdr, _ := splitHerdr(stopped); len(fromHerdr) == 0 {
		return nil
	}
	up, err := r.Serving(mc)
	if err != nil {
		return err
	}
	if up {
		return &HerdrRunning{Machine: mc.Label()}
	}
	return nil
}

// claimRecovery takes the one recovery of a machine at a time; false while another runs there.
func (r *Restarter) claimRecovery(machineID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recovering == nil {
		r.recovering = map[string]bool{}
	}
	if r.recovering[machineID] {
		return false
	}
	r.recovering[machineID] = true
	return true
}

func (r *Restarter) releaseRecovery(machineID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.recovering, machineID)
}

// ErrUnasked marks a check the machine could not answer — a dropped connection, a script that failed: nothing is known
// gone, so nothing is forgotten for it.
var ErrUnasked = errors.New("the machine could not be asked")

// CanResume says why a conversation cannot come back on the machine, nil when it can: its folder must still be there,
// and a Claude Code session's file too; a look the machine could not answer wraps ErrUnasked.
func CanResume(mc *machines.Machine, kind, session, dir string) error {
	info, err := mc.Stat(dir)
	switch {
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("%w: %v", ErrUnasked, err)
	case err != nil || info.Kind != "dir":
		return fmt.Errorf("its folder %s is gone — nothing to resume into", dir)
	}
	if kind != "claude" {
		return nil
	}
	switch found, err := sessionFileExists(mc, Chat{Kind: kind, Session: session, Dir: dir}); {
	case err != nil:
		return fmt.Errorf("%w: %v", ErrUnasked, err)
	case !found:
		return errors.New("its session file is gone — nothing to resume")
	}
	return nil
}

// sessionFileExists looks for the Claude Code session file where the agent writes it — a fresh stat of the folder's
// own file, then the reader's lookup across project folders (which remembers a miss for half a minute, so a file
// written after an early look still counts); an error when the machine could not be asked.
func sessionFileExists(mc *machines.Machine, c Chat) (bool, error) {
	own := mc.Home() + "/.claude/projects/" + transcript.EncodePath(c.Dir) + "/" + c.Session + ".jsonl"
	switch info, err := mc.Stat(own); {
	case err == nil && info.Kind != "dir":
		return true, nil
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return false, err
	}
	switch _, err := mc.Transcripts.ResolveSession(c.Kind, c.Dir, "id", c.Session); {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// starter is what resuming a chat needs of herdr's client.
type starter interface {
	SessionSnapshot() (*coreapi.Snapshot, error)
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

// notBack is every chat that did not come back: a start that failed, still listed, or one forgotten since it cannot.
func (os outcomes) notBack() (list []string) {
	for _, o := range os {
		if !o.back {
			list = append(list, o.title)
		}
	}
	return
}

// resume is the recovery over a client: a fresh look at each pane, the checks of the folder its shell stands in now,
// the start on the session.
func (r *Restarter) resume(client starter, machineID string, stopped []Stopped, checks func(Stopped, string) error, say func(string)) outcomes {
	var out outcomes
	for _, s := range stopped {
		out = append(out, r.resumeOne(client, machineID, s, checks, say))
	}
	return out
}

func (r *Restarter) resumeOne(client starter, machineID string, s Stopped, checks func(Stopped, string) error, say func(string)) outcome {
	if r.Lock != nil {
		l := r.Lock(machineID, s.Pane)
		l.Lock()
		defer l.Unlock()
	}
	a := agents[s.chat.Kind]
	settle := func(back bool, line string) outcome { return r.done(machineID, s, back, line, say) }
	snap, err := client.SessionSnapshot()
	if err != nil {
		return notStarted(s, fmt.Sprintf("· %s (%s): herdr did not answer: %v", s.Title, s.Pane, err), say)
	}
	p, found := Lookup(snap, s.Pane)
	switch {
	case !found:
		return settle(false, fmt.Sprintf("· %s (%s): the pane is gone", s.Title, s.Pane))
	case p.Agent == s.chat.Kind:
		return settle(true, fmt.Sprintf("· %s (%s): %s is back already", s.Title, s.Pane, a.Title))
	case p.Agent != "":
		return settle(false, fmt.Sprintf("· %s (%s): runs %q now — left as it is", s.Title, s.Pane, p.Agent))
	case !s.chat.in(p.Dir()):
		return settle(false, fmt.Sprintf("· %s (%s): the shell is in %s now, not %s — left as it is", s.Title, s.Pane, p.Dir(), s.chat.Dir))
	}
	if err := checks(s, p.Dir()); errors.Is(err, ErrUnasked) {
		return notStarted(s, fmt.Sprintf("· %s (%s): %v — still listed", s.Title, s.Pane, err), say)
	} else if err != nil {
		return settle(false, fmt.Sprintf("· %s (%s): %v", s.Title, s.Pane, err))
	}
	args := a.resume(s.chat.Session)
	say(fmt.Sprintf("· %s (%s): %s %s", s.Title, s.Pane, a.Kind, strings.Join(args, " ")))
	if err := startResumed(client, s.Pane, a.Kind, args); err != nil {
		return notStarted(s, fmt.Sprintf("  ✗ %s: %v — the pane is at its shell; run `%s` there", s.Title, err, Resume(a.Kind, s.chat.Session)), say)
	}
	return settle(true, fmt.Sprintf("  ✓ %s is back in %s, session %s resumed", a.Title, s.Title, short(s.chat.Session)))
}

// carryOn starts herdr's agents on remuda-core, each on its session in a tab of a workspace on its folder, else a new
// workspace named as herdr's was, so the agents of one herdr workspace in one folder become tabs of one; an agent whose
// session one of the core is on already counts as back, and a session twice on the list starts once.
func (r *Restarter) carryOn(client placer, machineID string, fromHerdr []Stopped, checks func(Stopped, string) error, say func(string)) outcomes {
	var out outcomes
	started := map[string]bool{}
	for _, s := range fromHerdr {
		out = append(out, r.carryOneOn(client, machineID, s, checks, started, say))
	}
	return out
}

func (r *Restarter) carryOneOn(client placer, machineID string, s Stopped, checks func(Stopped, string) error, started map[string]bool, say func(string)) outcome {
	a := agents[s.chat.Kind]
	if started[s.chat.Session] {
		return r.done(machineID, s, true, fmt.Sprintf("· %s (%s, from herdr): %s is on its session already", s.Title, s.Pane, a.Title), say)
	}
	dir := cmp.Or(s.chat.Ran, s.chat.Dir) // where the agent itself ran: its session is that folder's
	if err := checks(s, dir); errors.Is(err, ErrUnasked) {
		return notStarted(s, fmt.Sprintf("· %s (%s, from herdr): %v — still listed", s.Title, s.Pane, err), say)
	} else if err != nil {
		return r.done(machineID, s, false, fmt.Sprintf("· %s (%s, from herdr): %v", s.Title, s.Pane, err), say)
	}
	say(fmt.Sprintf("· %s (%s, from herdr): %s in %s", s.Title, s.Pane, Resume(a.Kind, s.chat.Session), dir))
	place, open, err := startOnSession(client, dir, s.Title, "", a.Kind, s.chat.Session)
	switch {
	case open != nil:
		return r.done(machineID, s, true, fmt.Sprintf("  ✓ %s is on its session already, in %s", a.Title, open.PaneID), say)
	case err != nil:
		return notStarted(s, fmt.Sprintf("  ✗ %s: %v", s.Title, err), say)
	}
	started[s.chat.Session] = true
	return r.done(machineID, s, true, fmt.Sprintf("  ✓ %s carries on in %s, session %s resumed", a.Title, place.Pane, short(s.chat.Session)), say)
}

// done is a stopped chat dealt with: off the list, and forgotten unless it is back.
func (r *Restarter) done(machineID string, s Stopped, back bool, line string, say func(string)) outcome {
	say(line)
	if r.Memory != nil {
		r.Memory.settle(machineID, s.Pane, back)
	}
	return outcome{title: s.Title, back: back, line: line}
}

// notStarted is a chat whose start failed: it stays on the list, for Resume to be pressed again.
func notStarted(s Stopped, line string, say func(string)) outcome {
	say(line)
	return outcome{title: s.Title, failed: true, line: line}
}

// paneOnSession is the pane whose agent is on the session, nil for none.
func paneOnSession(snap *coreapi.Snapshot, session string) *coreapi.PaneInfo {
	for i := range snap.Panes {
		if p := &snap.Panes[i]; sessionOf(p) == session {
			return p
		}
	}
	return nil
}

// startResumed starts the agent on its session under a name of the pane's own, waiting out a shell still coming up.
func startResumed(client starter, paneID, kind string, args []string) error {
	deadline := time.Now().Add(12 * time.Second)
	for {
		err := client.AgentStartWith(paneID, kind, agentName(kind, paneID), args, 90*time.Second)
		if err == nil || !(coreapi.IsCode(err, "agent_pane_busy") || coreapi.IsCode(err, "agent_not_ready")) || time.Now().After(deadline) {
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
