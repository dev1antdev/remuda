// Package arena hands one prompt to several agents at once — a task's prompt or a free one — each in a fresh git
// worktree of a repository workspace, on the machine the lane names, so their results can be read side by side and
// the best kept. An arena and its lanes are kept in the state dir (arenas.json); the lanes' agents are herdr's, as
// every agent is. Keeping a lane closes the others: each one's work, committed on its branch or not, is saved as a
// patch under the state dir first, then its workspace is closed and its worktree and branch removed.
package arena

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/state"
)

const fileName = "arenas.json"

// MinLanes and MaxLanes bound an arena; Keep is how many arenas the file holds, the oldest dropped first.
const (
	MinLanes = 2
	MaxLanes = 6
	Keep     = 30
)

// A lane's states: made and starting, its agent handed the prompt, refused on the way, kept by the person, closed with its work saved.
const (
	LaneStarting  = "starting"
	LaneRunning   = "running"
	LaneFailed    = "failed"
	LaneKept      = "kept"
	LaneDiscarded = "discarded"
)

// Lane is one agent of an arena: where it was asked to start, and where it did.
type Lane struct {
	ID        int    `json:"id"`
	Machine   string `json:"machine"`
	Workspace string `json:"workspace"` // the repository workspace its worktree branches off
	Agent     string `json:"agent"`
	Model     string `json:"model,omitempty"`

	Branch      string     `json:"branch,omitempty"`
	Dir         string     `json:"dir,omitempty"`          // the worktree's folder
	WorkspaceID string     `json:"workspace_id,omitempty"` // herdr's workspace for the worktree
	Pane        string     `json:"pane,omitempty"`
	Base        string     `json:"base,omitempty"` // the commit the worktree started from
	Started     *time.Time `json:"started,omitempty"`
	State       string     `json:"state"`
	Note        string     `json:"note,omitempty"`  // a refusal, or what became of the lane
	Patch       string     `json:"patch,omitempty"` // where its work was saved when it was closed
	RunID       int64      `json:"run_id,omitempty"`
}

// Arena is one prompt and its lanes.
type Arena struct {
	ID      int       `json:"id"`
	Title   string    `json:"title"`
	Prompt  string    `json:"prompt"`
	TaskID  int64     `json:"task_id,omitempty"`
	Created time.Time `json:"created"`
	Lanes   []Lane    `json:"lanes"`
	Kept    int       `json:"kept,omitempty"` // the lane kept, 0 while undecided
}

// Lane is the arena's lane by id.
func (a *Arena) Lane(id int) *Lane {
	for i := range a.Lanes {
		if a.Lanes[i].ID == id {
			return &a.Lanes[i]
		}
	}
	return nil
}

// ErrInvalid is a request the arena refuses before anything starts.
var ErrInvalid = errors.New("invalid")

func invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalid, msg) }

// LaneRequest is one lane as the page asks for it.
type LaneRequest struct {
	Machine   string `json:"machine"`
	Workspace string `json:"workspace"`
	Agent     string `json:"agent"`
	Model     string `json:"model"`
}

// Check refuses what could not start: too few or too many lanes, a lane without a machine or a workspace, a shell or an
// agent herdr does not know, a model its kind takes no flag for.
func Check(lanes []LaneRequest) error {
	if len(lanes) < MinLanes || len(lanes) > MaxLanes {
		return invalid(fmt.Sprintf("an arena has %d to %d lanes, not %d", MinLanes, MaxLanes, len(lanes)))
	}
	for i, l := range lanes {
		n := i + 1
		if strings.TrimSpace(l.Machine) == "" || strings.TrimSpace(l.Workspace) == "" {
			return invalid(fmt.Sprintf("lane %d needs a machine and a repository workspace", n))
		}
		if l.Agent == "" || l.Agent == "shell" || !sessions.KnownAgent(l.Agent) {
			return invalid(fmt.Sprintf("lane %d needs an agent, not %q", n, l.Agent))
		}
		if _, err := sessions.StartArgs(l.Agent, sessions.Start{Model: strings.TrimSpace(l.Model)}); err != nil {
			return invalid(fmt.Sprintf("lane %d: %v", n, err))
		}
	}
	return nil
}

// TitleOf is an arena's name: the prompt's first line, cut at a word by 80 characters.
func TitleOf(prompt string) string {
	line := strings.TrimSpace(prompt)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	runes := []rune(line)
	if len(runes) <= 80 {
		return line
	}
	cut := string(runes[:80])
	if i := strings.LastIndexByte(cut, ' '); i > 40 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-") + "…"
}

// BranchOf is a lane's branch: arena-<arena>-<lane>.
func BranchOf(arenaID, laneID int) string { return fmt.Sprintf("arena-%d-%d", arenaID, laneID) }

// Store keeps the arenas in the state dir.
type Store struct {
	dir string

	mu     sync.Mutex
	arenas []Arena // oldest first
	next   int
}

type file struct {
	Next   int     `json:"next"`
	Arenas []Arena `json:"arenas"`
}

// Open reads the arenas kept in dir; a damaged file is an empty list, said once.
func Open(dir string) *Store {
	st := &Store{dir: dir, next: 1}
	var f file
	if err := state.LoadJSON(dir, fileName, &f); err != nil {
		log.Printf("%s is unreadable (%v): no arena is remembered", fileName, err)
		return st
	}
	st.arenas = f.Arenas
	st.next = max(f.Next, 1)
	for _, a := range st.arenas {
		st.next = max(st.next, a.ID+1)
	}
	return st
}

func (st *Store) save() error {
	return state.SaveJSON(st.dir, fileName, file{Next: st.next, Arenas: st.arenas})
}

// Create keeps a new arena with its lanes starting, and gives it its id.
func (st *Store) Create(title, prompt string, taskID int64, lanes []LaneRequest, now time.Time) (Arena, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	a := Arena{ID: st.next, Title: title, Prompt: prompt, TaskID: taskID, Created: now}
	for i, l := range lanes {
		a.Lanes = append(a.Lanes, Lane{ID: i + 1, Machine: l.Machine, Workspace: l.Workspace, Agent: l.Agent, Model: strings.TrimSpace(l.Model), State: LaneStarting})
	}
	st.next++
	st.arenas = append(st.arenas, a)
	if len(st.arenas) > Keep {
		st.arenas = st.arenas[len(st.arenas)-Keep:]
	}
	return a, st.save()
}

// List is every arena, the newest first.
func (st *Store) List() []Arena {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]Arena, len(st.arenas))
	copy(out, st.arenas)
	for i := range out {
		out[i].Lanes = append([]Lane(nil), out[i].Lanes...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// Get is one arena, a copy.
func (st *Store) Get(id int) (Arena, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, a := range st.arenas {
		if a.ID == id {
			a.Lanes = append([]Lane(nil), a.Lanes...)
			return a, true
		}
	}
	return Arena{}, false
}

// Update changes one arena under the lock and keeps the file; false when there is no such arena.
func (st *Store) Update(id int, change func(*Arena)) (Arena, bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i := range st.arenas {
		if st.arenas[i].ID == id {
			change(&st.arenas[i])
			out := st.arenas[i]
			out.Lanes = append([]Lane(nil), out.Lanes...)
			return out, true, st.save()
		}
	}
	return Arena{}, false, nil
}

// SetLane changes one lane of an arena.
func (st *Store) SetLane(id, lane int, change func(*Lane)) error {
	_, _, err := st.Update(id, func(a *Arena) {
		if l := a.Lane(lane); l != nil {
			change(l)
		}
	})
	return err
}

// Forget drops an arena from the list; its lanes stay as they are.
func (st *Store) Forget(id int) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i := range st.arenas {
		if st.arenas[i].ID == id {
			st.arenas = append(st.arenas[:i], st.arenas[i+1:]...)
			return st.save()
		}
	}
	return nil
}
