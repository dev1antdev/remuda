package scenes

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/state"
)

// How an item of a run stands.
const (
	StateWaiting = "waiting" // not reached yet
	StateRunning = "running"
	StateStarted = "started" // a step's tab is there and its agent started
	StateDone    = "done"    // a tidy's action done
	StateRefused = "refused" // herdr, the agent or git said no; the note says why
	StateAway    = "away"    // its machine cannot be reached now
	StateSkipped = "skipped" // a tidy's action no longer applies; the note says why
)

// KeepRuns is how many runs stay in memory for the page to read.
const KeepRuns = 8

// ErrNoScene: the list holds no scene of that id.
var ErrNoScene = errors.New("no such scene")

// ErrBusy: a run of the scene is still on its way.
var ErrBusy = errors.New("the scene is running already")

// Run is one press of a scene and how each of its items went.
type Run struct {
	ID    string    `json:"id"`
	Scene string    `json:"scene"`
	Name  string    `json:"name"`
	Kind  string    `json:"kind"`
	At    time.Time `json:"at"`
	Done  bool      `json:"done"`
	Items []Item    `json:"items"`
}

// Item is one step or action of a run.
type Item struct {
	Index         int     `json:"index"`            // the step's place in the scene
	Step          *Step   `json:"step,omitempty"`   // a start scene's step as it ran
	Action        *Action `json:"action,omitempty"` // a tidy's action as it was planned
	State         string  `json:"state"`
	Note          string  `json:"note,omitempty"` // why it was refused, skipped or away
	Pane          string  `json:"pane,omitempty"` // a started step's pane, to open
	MadeWorkspace bool    `json:"made_workspace,omitempty"`
}

// Service runs the scenes: the steps of a start scene one after another, a tidy's actions after a fresh look.
type Service struct {
	Store    *Store
	Recipe   func(id string) (recipes.Recipe, bool)
	Machines func() []Machine // the fleet, connected or not
	Machine  func(id string) (Machine, error)
	Readings Readings
	Input    *sessions.Input
	Notify   func(push.Payload)                   // a first prompt not delivered
	Sent     func(by, machine, pane, text string) // a /compact typed: a quiet row of the activity
	Now      func() time.Time

	mu   sync.Mutex
	runs []*Run
	busy map[string]bool
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Start begins a run of the scene in the background: the steps picked by their index (every one for none) of a start
// scene, or the actions picked by their id of a tidy, which are looked at again first. by names who pressed it.
func (s *Service) Start(sceneID string, steps []int, actions []string, by string) (Run, error) {
	scene, ok := s.Store.Get(sceneID)
	if !ok {
		return Run{}, ErrNoScene
	}
	run := &Run{ID: state.NewID(), Scene: scene.ID, Name: scene.Name, Kind: scene.Kind, At: s.now()}
	if scene.Kind == KindTidy {
		if len(actions) == 0 {
			return Run{}, errors.New("pick what to tidy")
		}
		for i := range actions {
			run.Items = append(run.Items, Item{Index: i, Action: &Action{ID: actions[i]}, State: StateWaiting})
		}
	} else {
		picked, err := pickSteps(scene.Steps, steps)
		if err != nil {
			return Run{}, err
		}
		for _, i := range picked {
			step := scene.Steps[i]
			run.Items = append(run.Items, Item{Index: i, Step: &step, State: StateWaiting})
		}
	}
	if !s.claim(scene.ID, run) {
		return Run{}, ErrBusy
	}
	go func() {
		defer s.release(scene.ID, run)
		s.execute(scene, run, by)
	}()
	return s.copyOf(run), nil
}

// RunNow runs every step of a start scene, or everything a tidy's plan holds, and answers once it is done: a schedule's run.
func (s *Service) RunNow(sceneID, by string) (Run, error) {
	scene, ok := s.Store.Get(sceneID)
	if !ok {
		return Run{}, errors.New("the scene it runs is gone")
	}
	run := &Run{ID: state.NewID(), Scene: scene.ID, Name: scene.Name, Kind: scene.Kind, At: s.now()}
	if scene.Kind == KindTidy {
		for i, action := range s.Plan(*scene.Tidy, s.now()).Actions {
			action := action
			run.Items = append(run.Items, Item{Index: i, Action: &action, State: StateWaiting})
		}
	} else {
		for i := range scene.Steps {
			step := scene.Steps[i]
			run.Items = append(run.Items, Item{Index: i, Step: &step, State: StateWaiting})
		}
	}
	if !s.claim(scene.ID, run) {
		return Run{}, ErrBusy
	}
	defer s.release(scene.ID, run)
	s.execute(scene, run, by)
	return s.copyOf(run), nil
}

// pickSteps is the steps asked for by their index, each once and in the scene's order; none asked is every step.
func pickSteps(steps []Step, asked []int) ([]int, error) {
	if len(asked) == 0 {
		all := make([]int, len(steps))
		for i := range all {
			all[i] = i
		}
		return all, nil
	}
	chosen := map[int]bool{}
	for _, i := range asked {
		if i < 0 || i >= len(steps) {
			return nil, fmt.Errorf("the scene has no step %d", i+1)
		}
		chosen[i] = true
	}
	var out []int
	for i := range steps {
		if chosen[i] {
			out = append(out, i)
		}
	}
	return out, nil
}

func (s *Service) claim(sceneID string, run *Run) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy == nil {
		s.busy = map[string]bool{}
	}
	if s.busy[sceneID] {
		return false
	}
	s.busy[sceneID] = true
	s.runs = append(s.runs, run)
	if len(s.runs) > KeepRuns {
		s.runs = s.runs[len(s.runs)-KeepRuns:]
	}
	return true
}

func (s *Service) release(sceneID string, run *Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.busy, sceneID)
	run.Done = true
}

// Get is a run as it stands, by its id.
func (s *Service) Get(id string) (Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		if run.ID == id {
			return copyLocked(run), true
		}
	}
	return Run{}, false
}

func (s *Service) copyOf(run *Run) Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyLocked(run)
}

func copyLocked(run *Run) Run {
	out := *run
	out.Items = append([]Item(nil), run.Items...)
	return out
}

// update changes one item of a run under the lock, so a read of the run never sees it half written.
func (s *Service) update(run *Run, i int, change func(*Item)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(&run.Items[i])
}

func (s *Service) execute(scene Scene, run *Run, by string) {
	if scene.Kind == KindTidy {
		s.tidy(*scene.Tidy, run, by)
		return
	}
	for i := range run.Items {
		s.update(run, i, func(item *Item) { item.State = StateRunning })
		s.mu.Lock()
		step := *run.Items[i].Step
		s.mu.Unlock()
		var outcome Item
		s.runStep(scene.Name, step, &outcome)
		s.update(run, i, func(item *Item) {
			item.State, item.Note, item.Pane, item.MadeWorkspace = outcome.State, outcome.Note, outcome.Pane, outcome.MadeWorkspace
		})
	}
}

// firstPrompt hands a step's prompt to the agent it just started; one not delivered is a note to the person with the pane to open.
func (s *Service) firstPrompt(client sessions.Prompter, machine, pane, prompt, sceneName string) {
	err := s.Input.FirstPrompt(client, events.PaneRef{Machine: machine, Pane: pane}, prompt, sessions.FirstPromptTimeout)
	if err == nil || s.Notify == nil {
		return
	}
	s.Notify(push.Payload{
		Machine: machine, Pane: pane, Kind: "input",
		Title:   "Scene “" + sceneName + "”: the first prompt was not delivered",
		Body:    err.Error(),
		Tag:     "first|" + pane,
		Actions: []push.Action{{Action: "open", Title: "Open"}},
	})
}

// Summary is what a run came to in one line — how many items went each way — and an error naming the first refusal
// when one was refused.
func Summary(run Run) (string, error) {
	counts := map[string]int{}
	refusal := ""
	for _, item := range run.Items {
		counts[item.State]++
		if item.State == StateRefused && refusal == "" {
			refusal = item.Note
		}
	}
	var parts []string
	for _, state := range []string{StateStarted, StateDone, StateAway, StateSkipped, StateRefused} {
		if n := counts[state]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, state))
		}
	}
	line := strings.Join(parts, ", ")
	if line == "" {
		line = "nothing to do"
	}
	if refusal != "" {
		return "", fmt.Errorf("%s: %s", line, refusal)
	}
	return line, nil
}
