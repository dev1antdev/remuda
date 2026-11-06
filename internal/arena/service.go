package arena

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/watch"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// Deliver paces the prompt's handover to a lane's fresh agent: a try every DeliverRetry until DeliverTimeout.
var (
	DeliverRetry   = 2 * time.Second
	DeliverTimeout = 5 * time.Minute
	StatsEvery     = 15 * time.Second
	ScriptTimeout  = 60 * time.Second
)

// Service starts arenas and closes the lanes the person did not keep.
type Service struct {
	Store    *Store
	Machines *machines.Manager
	Watch    *watch.Watcher
	Input    *sessions.Input
	Tasks    *tasks.Store
	Notify   func(push.Payload)
	Dir      string // the state dir; the patches go under its arenas/
	Now      func() time.Time

	mu    sync.Mutex
	stats map[int]statsRead   // by arena
	locks map[int]*sync.Mutex // by arena: a lane made, or the arena decided, one at a time
}

// arenaLock is the arena's own lock, held while one of its lanes is made and while the person's Keep closes the rest,
// so a lane is never made after the arena was decided nor closed half made.
func (s *Service) arenaLock(id int) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks == nil {
		s.locks = map[int]*sync.Mutex{}
	}
	lock, ok := s.locks[id]
	if !ok {
		lock = &sync.Mutex{}
		s.locks[id] = lock
	}
	return lock
}

// stillStarting says whether a lane is still to be made: the arena there and undecided, the lane not closed meanwhile.
func (s *Service) stillStarting(arenaID, laneID int) bool {
	a, ok := s.Store.Get(arenaID)
	if !ok || a.Kept != 0 {
		return false
	}
	lane := a.Lane(laneID)
	return lane != nil && lane.State == LaneStarting
}

type statsRead struct {
	at    time.Time
	lanes map[int]Stat
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Request is a new arena as the page asks for it: a prompt, or a task whose prompt it becomes, and its lanes.
type Request struct {
	Prompt string        `json:"prompt"`
	TaskID int64         `json:"task_id"`
	Groom  bool          `json:"groom"`
	Lanes  []LaneRequest `json:"lanes"`
}

// Start checks the request, keeps the arena and starts its lanes in the background, one after another; it answers
// at once with every lane starting.
func (s *Service) Start(req Request) (Arena, error) {
	if err := Check(req.Lanes); err != nil {
		return Arena{}, err
	}
	for _, l := range req.Lanes {
		if _, err := s.Machines.Get(l.Machine); err != nil {
			return Arena{}, invalid(err.Error())
		}
	}
	prompt := strings.TrimSpace(req.Prompt)
	title := TitleOf(prompt)
	if req.TaskID != 0 {
		task, err := s.Tasks.Get(req.TaskID)
		if err != nil {
			return Arena{}, err
		}
		if prompt == "" {
			project, _ := s.Tasks.Project(task.ProjectID)
			prompt = tasks.Prompt(task, project, req.Groom)
		}
		title = task.Identifier + " " + task.Title
	}
	if prompt == "" {
		return Arena{}, invalid("an arena needs a prompt or a task")
	}
	a, err := s.Store.Create(title, prompt, req.TaskID, req.Lanes, s.now())
	if err != nil {
		return Arena{}, err
	}
	go s.startLanes(a)
	return a, nil
}

// startLanes makes each lane's worktree and starts its agent, one at a time so one repository's worktrees are not
// made side by side; a lane refused on the way says why and the others go on.
func (s *Service) startLanes(a Arena) {
	lock := s.arenaLock(a.ID)
	for _, lane := range a.Lanes {
		lock.Lock()
		if s.stillStarting(a.ID, lane.ID) {
			if err := s.startLane(a, lane); err != nil {
				s.settle(a.ID, lane.ID, LaneFailed, err.Error())
			}
		}
		lock.Unlock()
	}
	if a.TaskID != 0 {
		_, _ = s.Tasks.AddComment(a.TaskID, "remuda", fmt.Sprintf("Arena #%d: the task went to %d agents side by side, each in a worktree of its own.", a.ID, len(a.Lanes)))
	}
}

func (s *Service) startLane(a Arena, lane Lane) error {
	mc, err := s.Machines.Get(lane.Machine)
	if err != nil {
		return err
	}
	client, _, err := mc.Ready()
	if err != nil {
		return err
	}
	label := fmt.Sprintf("arena %d · %s", a.ID, lane.Agent)
	res, err := client.WorktreeCreate(lane.Workspace, BranchOf(a.ID, lane.ID), "", label)
	if err != nil {
		return fmt.Errorf("the worktree was not made: %w", err)
	}
	pane := res.RootPane.PaneID
	base, _ := shell.Text(mc, HeadScript(res.Worktree.Path), ScriptTimeout)
	started := s.now()
	_ = s.Store.SetLane(a.ID, lane.ID, func(l *Lane) {
		l.Branch, l.Dir, l.WorkspaceID, l.Pane = res.Worktree.Branch, res.Worktree.Path, res.Workspace.WorkspaceID, pane
		l.Base = strings.TrimSpace(base)
		l.Started = &started
	})
	if err := sessions.StartAgentWith(client, pane, lane.Agent, sessions.Start{Model: lane.Model}); err != nil {
		s.Watch.Refresh(mc)
		return fmt.Errorf("the worktree %s is there, but the agent did not start: %w", res.Worktree.Branch, err)
	}
	s.Watch.Refresh(mc)
	if a.TaskID != 0 {
		if run, err := s.Tasks.StartRun(tasks.Run{TaskID: a.TaskID, Machine: mc.ID(), Workspace: res.Workspace.WorkspaceID, Pane: pane, Dir: res.Worktree.Path, Branch: res.Worktree.Branch, Agent: lane.Agent}); err == nil {
			_ = s.Store.SetLane(a.ID, lane.ID, func(l *Lane) { l.RunID = run.ID })
		}
	}
	go s.deliver(mc, client, a, lane.ID, pane, res.Worktree.Path)
	return nil
}

// deliver hands the prompt to a lane's fresh agent; Claude Code's "trust this folder" question for that very worktree
// is answered yes, as a task's start does, any other question is pushed to the person once and waited for.
func (s *Service) deliver(mc *machines.Machine, client *herdr.Client, a Arena, laneID int, pane, dir string) {
	lock := s.Input.Lock(mc.ID(), pane)
	deadline := s.now().Add(DeliverTimeout)
	nudged, trusted := false, false
	var last error
	for s.now().Before(deadline) {
		time.Sleep(DeliverRetry)
		lock.Lock()
		err := client.AgentPrompt(pane, a.Prompt)
		lock.Unlock()
		switch {
		case err == nil:
			s.settle(a.ID, laneID, LaneRunning, "")
			return
		case herdr.IsCode(err, "agent_not_ready") || herdr.IsCode(err, "agent_not_found"):
			last = err
		case herdr.IsCode(err, "agent_blocked"):
			last = err
			if !trusted {
				if screen, rerr := client.PaneRead(pane); rerr == nil && tasks.TrustPrompt(screen, dir, mc.Home()) {
					trusted = true
					lock.Lock()
					if !tasks.CursorOnYes(screen) {
						_ = client.PaneSendKeys(pane, []string{"down"})
						time.Sleep(150 * time.Millisecond)
					}
					_ = client.PaneSendKeys(pane, []string{"enter"})
					lock.Unlock()
					continue
				}
			}
			if !nudged && s.Notify != nil {
				nudged = true
				s.Notify(push.Payload{
					Machine: mc.ID(), Pane: pane, Kind: "input",
					Title:   fmt.Sprintf("Arena #%d: an agent asks something first", a.ID),
					Body:    "Answer it in the pane and the prompt follows.",
					Tag:     fmt.Sprintf("arena|%d|%d", a.ID, laneID),
					Actions: []push.Action{{Action: "open", Title: "Open"}},
				})
			}
		default:
			s.settle(a.ID, laneID, LaneFailed, "the prompt was refused: "+err.Error())
			return
		}
	}
	note := "the agent did not take the prompt within five minutes"
	if last != nil {
		note += " (" + last.Error() + ")"
	}
	s.settle(a.ID, laneID, LaneFailed, note+"; the worktree is there, send it by hand")
}

// settle moves a lane out of starting; a lane the person kept or closed meanwhile stays as it is.
func (s *Service) settle(arenaID, laneID int, state, note string) {
	_ = s.Store.SetLane(arenaID, laneID, func(l *Lane) {
		if l.State == LaneStarting {
			l.State, l.Note = state, note
		}
	})
}

// Stats is every lane's work against its base, read on each lane's machine at most every StatsEvery.
func (s *Service) Stats(a Arena) map[int]Stat {
	s.mu.Lock()
	if s.stats == nil {
		s.stats = map[int]statsRead{}
	}
	read, ok := s.stats[a.ID]
	s.mu.Unlock()
	if ok && s.now().Sub(read.at) < StatsEvery {
		return read.lanes
	}
	byMachine := map[string][]Lane{}
	for _, l := range a.Lanes {
		if l.State != LaneDiscarded && l.Dir != "" && l.Base != "" {
			byMachine[l.Machine] = append(byMachine[l.Machine], l)
		}
	}
	lanes := map[int]Stat{}
	for id, list := range byMachine {
		mc, err := s.Machines.Get(id)
		if err != nil {
			continue
		}
		if now, _ := mc.State(); now != machines.StateConnected {
			continue
		}
		out, err := shell.Text(mc, StatsScript(list), ScriptTimeout)
		if err != nil && out == "" {
			continue
		}
		for lane, stat := range ParseStats(out) {
			lanes[lane] = stat
		}
	}
	s.mu.Lock()
	s.stats[a.ID] = statsRead{at: s.now(), lanes: lanes}
	s.mu.Unlock()
	return lanes
}

// ErrNotFound is an arena or a lane that is not there.
var ErrNotFound = errors.New("not found")

// Keep keeps one lane and closes every other: its work saved as a patch under the state dir, its workspace closed, its
// worktree and branch removed. A lane whose work could not be saved, or whose machine is away, stays as it is, said in
// its note, and keeping the same lane again tries it once more; a lane still being made is waited for, and the ones
// not made yet never are. The answer is the arena as it now stands.
func (s *Service) Keep(id, laneID int) (Arena, error) {
	lock := s.arenaLock(id)
	lock.Lock()
	defer lock.Unlock()
	a, ok := s.Store.Get(id)
	if !ok || a.Lane(laneID) == nil {
		return Arena{}, ErrNotFound
	}
	switch kept := a.Lane(laneID); {
	case a.Kept != 0 && a.Kept != laneID:
		return Arena{}, invalid(fmt.Sprintf("lane %d was kept already", a.Kept))
	case kept.State == LaneDiscarded:
		return Arena{}, invalid("that lane is closed already")
	case kept.Dir == "":
		return Arena{}, invalid("that lane has no worktree to keep")
	}
	var saved []string
	for _, lane := range a.Lanes {
		if lane.ID == laneID || lane.State == LaneDiscarded {
			continue
		}
		patch, note := s.discard(a, lane)
		_ = s.Store.SetLane(a.ID, lane.ID, func(l *Lane) {
			if note == "" {
				l.State, l.Patch, l.Note = LaneDiscarded, patch, ""
				return
			}
			l.Note = note
			if patch != "" {
				l.Patch = patch
			}
		})
		if patch != "" {
			saved = append(saved, patch)
		}
	}
	out, _, err := s.Store.Update(a.ID, func(x *Arena) {
		x.Kept = laneID
		if l := x.Lane(laneID); l != nil {
			l.State = LaneKept
		}
	})
	if a.TaskID != 0 && a.Kept == 0 {
		kept := a.Lane(laneID)
		msg := fmt.Sprintf("Arena #%d: kept %s%s in `%s`.", a.ID, kept.Agent, modelWords(kept.Model), kept.Dir)
		if len(saved) > 0 {
			msg += " The other lanes' work is saved as patches: `" + strings.Join(saved, "`, `") + "`."
		}
		_, _ = s.Tasks.AddComment(a.TaskID, "remuda", msg)
	}
	return out, err
}

func modelWords(model string) string {
	if model == "" {
		return ""
	}
	return " (" + model + ")"
}

// discard closes one lane: the patch first, then its workspace, then its worktree; the note says what stopped it.
func (s *Service) discard(a Arena, lane Lane) (patch, note string) {
	if lane.Dir == "" && lane.WorkspaceID == "" {
		return "", "" // it never started: nothing to close
	}
	if lane.Dir != "" && lane.Base == "" { // the commit it started from was never read: no patch could hold its work
		return "", "kept: its work could not be saved (the commit it started from is not known)"
	}
	mc, err := s.Machines.Get(lane.Machine)
	if err != nil {
		return "", "kept: " + err.Error()
	}
	client, _, err := mc.Ready()
	if err != nil {
		return "", "kept: the machine is away (" + err.Error() + ")"
	}
	if lane.Dir != "" && lane.Base != "" {
		out, err := shell.Text(mc, PatchScript(lane.Dir, lane.Base), ScriptTimeout)
		if err != nil {
			return "", "kept: its work could not be saved (" + lastLine(out, err.Error()) + ")"
		}
		patch, err = s.savePatch(a, lane, out)
		if err != nil {
			return "", "kept: its work could not be saved (" + err.Error() + ")"
		}
	}
	if lane.WorkspaceID != "" {
		if err := client.WorkspaceClose(lane.WorkspaceID); err != nil && !herdr.IsCode(err, "workspace_not_found") {
			return patch, "kept: herdr did not close its workspace (" + err.Error() + ")"
		}
		s.Watch.Refresh(mc)
	}
	if lane.Dir == "" {
		return patch, ""
	}
	info, err := worktrees.Inspect(mc, lane.Dir)
	if err != nil {
		return patch, "its workspace is closed; the worktree stays: " + err.Error()
	}
	if err := worktrees.Discard(mc, ownBranch(info, lane)); err != nil {
		return patch, "its workspace is closed; git kept the worktree: " + err.Error()
	}
	return patch, ""
}

// ownBranch is the worktree as it is to be discarded: the branch the arena made for the lane deleted, never the one
// the lane's agent may have checked out there since, which is the person's.
func ownBranch(info worktrees.Info, lane Lane) worktrees.Info {
	info.Branch = lane.Branch
	return info
}

// savePatch writes a lane's work to <state>/arenas/arena-<id>-lane-<n>-<agent>.patch, 0600; an empty work is no file.
func (s *Service) savePatch(a Arena, lane Lane, diff string) (string, error) {
	if strings.TrimSpace(diff) == "" {
		return "", nil
	}
	dir := filepath.Join(s.Dir, "arenas")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("arena-%d-lane-%d-%s.patch", a.ID, lane.ID, lane.Agent))
	return path, state.WriteFile(path, []byte(diff))
}

// lastLine is a script's complaint: its last printed line, else the fallback.
func lastLine(text, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	if i := strings.LastIndex(text, "\n"); i >= 0 {
		return strings.TrimSpace(text[i+1:])
	}
	return text
}
