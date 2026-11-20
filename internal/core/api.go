package core

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// callWithin bounds one request the adapter sends on the watcher's or a handler's behalf, as herdr's client does.
var callWithin = 10 * time.Second

// maxQueued is the events a stream holds for a reader that fell behind; past it the stream ends as herdr's does,
// with events_lost, and the reader takes a snapshot.
const maxQueued = 4096

// outputOnly is a label no terminal carries: the subscription that holds a stream's output watches filters every
// terminal out with it, so it tells the watches' rows and nothing else.
const outputOnly = "remuda.output-only"

// eventStream is one herdr event stream over the adapter: the mirror's changes as they are applied, and the rows its
// output watches matched, through a core subscription of its own.
type eventStream struct {
	adapter *Adapter
	wake    chan struct{}
	output  *Subscription
	panes   map[string]string // output watch id → pane

	mu    sync.Mutex
	queue []herdr.Event
	err   error
}

// Subscribe opens a stream: every change of the machine, whatever the subscriptions name, and the output matches they
// ask for. A refusal of the watches is herdr's (invalid_regex, pane_not_found).
func (a *Adapter) Subscribe(subs []herdr.Subscription) (herdr.EventStream, error) {
	s := &eventStream{adapter: a, wake: make(chan struct{}, 1), panes: map[string]string{}}
	var watches []OutputWatch
	for i, sub := range subs {
		if sub.Type != herdr.EventOutputMatched || sub.Match == nil {
			continue
		}
		id := strconv.Itoa(i)
		s.panes[id] = sub.PaneID
		watches = append(watches, OutputWatch{ID: id, Terminal: sub.PaneID, Match: Match{Type: sub.Match.Type, Value: sub.Match.Value}, Lines: min(sub.Lines, 200)})
	}
	if len(watches) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), callWithin)
		output, err := a.client.Subscribe(ctx, SubscribeParams{Labels: map[string]string{outputOnly: "1"}, Output: watches})
		cancel()
		if err != nil {
			return nil, a.refusal(err, "pane_not_found")
		}
		s.output = output
	}
	a.mu.Lock()
	if !a.held {
		err := a.whyNotHeld()
		a.mu.Unlock()
		s.closeOutput()
		return nil, err
	}
	a.streams[s] = struct{}{}
	a.mu.Unlock()
	if s.output != nil {
		go s.readOutput()
	}
	return s, nil
}

func (s *eventStream) Next() (herdr.Event, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			ev := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return ev, nil
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			return herdr.Event{}, err
		}
		<-s.wake
	}
}

// Close ends the stream; Next answers net.ErrClosed, as a herdr stream closed here does.
func (s *eventStream) Close() error {
	s.adapter.mu.Lock()
	delete(s.adapter.streams, s)
	s.adapter.mu.Unlock()
	s.end(net.ErrClosed)
	s.closeOutput()
	return nil
}

func (s *eventStream) closeOutput() {
	if s.output != nil {
		s.output.Close()
	}
}

func (s *eventStream) push(ev herdr.Event) {
	s.mu.Lock()
	if s.err == nil {
		if len(s.queue) < maxQueued {
			s.queue = append(s.queue, ev)
		} else {
			s.queue, s.err = nil, &herdr.Error{Code: herdr.ErrEventsLost, Message: "the reader fell behind"}
		}
	}
	s.mu.Unlock()
	s.ring()
}

func (s *eventStream) end(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
	s.ring()
}

func (s *eventStream) ring() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// readOutput hands the stream every row its watches matched, as herdr's pane.output_matched.
func (s *eventStream) readOutput() {
	for {
		notice, err := s.output.Next(context.Background())
		if err != nil {
			s.end(err)
			return
		}
		if notice.Kind != NoticeEvent || notice.Event.Type != "output" {
			continue
		}
		var row struct {
			Watch string `json:"watch"`
			Line  string `json:"line"`
		}
		if json.Unmarshal(notice.Event.Data, &row) != nil {
			continue
		}
		s.push(herdr.Event{Kind: herdr.EventOutputMatched, PaneID: s.panes[row.Watch], MatchedLine: row.Line, Text: row.Line})
	}
}

// Bridged is false: a stream costs the machine nothing, the mirror's changes are fanned out here.
func (a *Adapter) Bridged() bool { return false }

func (a *Adapter) read(paneID string, lines int, format string) (ReadResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	read, err := a.client.Read(ctx, paneID, lines, format)
	return read, a.refusal(err, "pane_not_found")
}

// PaneRead is the terminal's screen as text.
func (a *Adapter) PaneRead(paneID string) (string, error) {
	read, err := a.read(paneID, 0, "")
	return read.Text, err
}

// PaneScreen is the terminal's screen with its colours, as SGR.
func (a *Adapter) PaneScreen(paneID string) (herdr.Screen, error) {
	read, err := a.read(paneID, 0, "vt")
	return herdr.Screen{Text: read.Text}, err
}

// PaneRecent is the terminal's last n rows down to the last with text, the screen and the history above it, and
// whether rows before them are gone.
func (a *Adapter) PaneRecent(paneID string, n int) (string, bool, error) {
	read, err := a.read(paneID, n, "")
	if err != nil {
		return "", false, err
	}
	rows := strings.Split(strings.TrimRight(read.Text, "\n "), "\n")
	return strings.Join(rows[max(0, len(rows)-n):], "\n"), !read.Complete, nil
}

// terminal is a copy of the mirror's record of a pane.
func (a *Adapter) terminal(paneID string) (Terminal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.held {
		return Terminal{}, a.whyNotHeld()
	}
	t := a.state.terminals[paneID]
	if t == nil {
		return Terminal{}, &herdr.Error{Code: "pane_not_found", Message: "no terminal " + paneID}
	}
	return *t, nil
}

// PaneAtPrompt says whether the terminal's own shell holds it: nothing the person started runs in its foreground.
func (a *Adapter) PaneAtPrompt(paneID string) (bool, error) {
	t, err := a.terminal(paneID)
	if err != nil {
		return false, err
	}
	fg := t.Foreground
	return fg != nil && fg.Pid == t.Pid && shells[programName(fg.Name)], nil
}

// PaneShellPID is the process the terminal started, the root of what runs in it.
func (a *Adapter) PaneShellPID(paneID string) (int, error) {
	t, err := a.terminal(paneID)
	if err == nil && t.Pid == 0 {
		err = errors.New("terminal " + paneID + " runs no process now")
	}
	return t.Pid, err
}

// AgentGet is the agent in a pane and how it stands, from the mirror.
func (a *Adapter) AgentGet(target string) (herdr.AgentInfo, error) {
	t, err := a.terminal(target)
	if err != nil {
		return herdr.AgentInfo{}, err
	}
	agent := agentOf(&t)
	if agent == "" {
		return herdr.AgentInfo{}, &herdr.Error{Code: "agent_not_found", Message: "no agent in " + target}
	}
	return herdr.AgentInfo{Agent: agent, Status: statusOf(&t)}, nil
}

func (a *Adapter) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	return nil, notOnCore("workspace.create")
}
func (a *Adapter) WorkspaceRename(workspaceID, label string) error {
	return notOnCore("workspace.rename")
}
func (a *Adapter) WorkspaceClose(workspaceID string) error { return notOnCore("workspace.close") }
func (a *Adapter) WorkspaceReportMetadata(m herdr.WorkspaceMetadata) error {
	return notOnCore("workspace.report_metadata")
}
func (a *Adapter) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	return nil, notOnCore("tab.create")
}
func (a *Adapter) TabCreateIn(workspaceID, label, cwd string, env map[string]string) (*herdr.TabCreateResult, error) {
	return nil, notOnCore("tab.create")
}
func (a *Adapter) TabRename(tabID, label string) error { return notOnCore("tab.rename") }
func (a *Adapter) TabClose(tabID string) error         { return notOnCore("tab.close") }
func (a *Adapter) WorktreeCreate(workspaceID, branch, base, label string) (*herdr.WorktreeCreateResult, error) {
	return nil, notOnCore("worktree.create")
}
func (a *Adapter) WorktreeCreateAt(workspaceID, branch, base, label, path string) (*herdr.WorktreeCreateResult, error) {
	return nil, notOnCore("worktree.create")
}
func (a *Adapter) WorktreeOpen(workspaceID, path, label string) (*herdr.WorktreeCreateResult, error) {
	return nil, notOnCore("worktree.open")
}
func (a *Adapter) LayoutExport(tabID string) (*herdr.LayoutDescription, error) {
	return nil, notOnCore("layout.export")
}
func (a *Adapter) LayoutApply(workspaceID, tabID, tabLabel string, root herdr.LayoutNode) (*herdr.LayoutDescription, error) {
	return nil, notOnCore("layout.apply")
}
func (a *Adapter) PaneSplit(paneID, direction, cwd string) (*herdr.PaneInfo, error) {
	return nil, notOnCore("pane.split")
}
func (a *Adapter) PaneClose(paneID string) error { return notOnCore("pane.close") }
func (a *Adapter) PaneScroll(paneID string, offset int) (herdr.Scroll, error) {
	return herdr.Scroll{}, notOnCore("pane.scroll")
}
func (a *Adapter) PaneSendText(paneID, text string) error { return notOnCore("pane.send_text") }
func (a *Adapter) PaneSendKeys(paneID string, keys []string) error {
	return notOnCore("pane.send_keys")
}
func (a *Adapter) PaneSendInput(paneID, text string, keys []string) error {
	return notOnCore("pane.send_input")
}
func (a *Adapter) PaneReportMetadata(m herdr.PaneMetadata) error {
	return notOnCore("pane.report_metadata")
}
func (a *Adapter) AgentStart(paneID, kind string) error { return notOnCore("agent.start") }
func (a *Adapter) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	return notOnCore("agent.start")
}
func (a *Adapter) AgentPrompt(target, text string) error { return notOnCore("agent.prompt") }
func (a *Adapter) NotificationShow(t herdr.Toast) (herdr.Shown, error) {
	return herdr.Shown{}, notOnCore("notification.show")
}
