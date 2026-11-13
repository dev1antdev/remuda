// Package scenes keeps the person's scenes, one JSON list in the state dir replaced whole: a start scene is the set-up
// of a morning — workspaces and tabs on folders across the machines, each with an agent (or a recipe, or a shell), its
// model, arguments, first prompt, tab name and optionally a tab's layout captured from herdr — run step after step at
// the person's press; a tidy scene is the evening's — the shells at their prompts closed, agents idle for hours closed
// (their sessions stay resumable), /compact sent to idle Claude Codes past a context share, clean leftover worktrees
// removed — each shown before it runs, never touching an agent that works or asks.
package scenes

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/state"
)

const file = "scenes.json"

// The kinds of scene: steps that start things, or a tidy of what stands.
const (
	KindStart = "start"
	KindTidy  = "tidy"
)

// The limits a scene is checked against.
const (
	MaxSteps     = 12
	MaxName      = 60  // runes of a scene's name
	MaxTab       = 40  // runes of a tab's name or a pane's label
	MaxIdleHours = 168 // a week
)

// The tidy's defaults, used when a choice is switched on without its number.
const (
	DefaultIdleHours = 8
	DefaultCompactAt = 70
)

// Scene is one named set-up or tidy.
type Scene struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`            // start (the default) | tidy
	Steps []Step `json:"steps,omitempty"` // a start scene's, run one after another
	Tidy  *Tidy  `json:"tidy,omitempty"`  // a tidy scene's choices
}

// Step is a workspace or tab a start scene makes on a folder of a machine, and what runs in it.
type Step struct {
	Machine string  `json:"machine"`
	Dir     string  `json:"dir"`
	Agent   string  `json:"agent,omitempty"`  // a kind herdr knows, "shell" for a plain shell; "" when the recipe names it
	Recipe  string  `json:"recipe,omitempty"` // a recipe's id: its agent, model, arguments and prompt, read when the step runs
	Model   string  `json:"model,omitempty"`  // as the agent's own CLI names it, "" for its default
	Args    string  `json:"args,omitempty"`   // extra arguments as the person typed them
	Prompt  string  `json:"prompt,omitempty"` // handed over once the agent is up
	Tab     string  `json:"tab,omitempty"`    // the tab's name, "" for herdr's own
	Layout  *Layout `json:"layout,omitempty"` // a tab's panes as captured, "" for one pane
}

// Tidy is what an evening scene does when the person presses it, each switch on its own.
type Tidy struct {
	Shells    bool     `json:"shells"`               // close the shells sitting at their prompt
	Idle      bool     `json:"idle"`                 // close agents idle for IdleHours or more
	IdleHours int      `json:"idle_hours,omitempty"` // 1 to a week
	Compact   bool     `json:"compact"`              // send /compact to idle Claude Codes past CompactAt
	CompactAt int      `json:"compact_at,omitempty"` // percent of the context window, 10 to 95
	Worktrees bool     `json:"worktrees"`            // remove the clean leftover worktrees
	Machines  []string `json:"machines,omitempty"`   // the machines it tidies, none for every one
}

// Store is the list in the state dir.
type Store struct {
	dir string
	// Recipe says whether a recipe id is one the cookbook has; nil takes any
	Recipe func(id string) bool

	mu  sync.Mutex
	all []Scene
}

func New(dir string) *Store {
	st := &Store{dir: dir}
	if err := state.LoadJSON(dir, file, &st.all); err != nil {
		log.Printf("%s is unreadable (%v): no scenes until it is fixed", file, err)
	}
	if st.all == nil {
		st.all = []Scene{}
	}
	return st
}

func (st *Store) List() []Scene {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append(make([]Scene, 0, len(st.all)), st.all...)
}

// Get is one scene by its id.
func (st *Store) Get(id string) (Scene, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, s := range st.all {
		if s.ID == id {
			return s, true
		}
	}
	return Scene{}, false
}

// Replace keeps the whole list as given, each scene checked and trimmed; ids the client did not send, or sent twice,
// are made up here.
func (st *Store) Replace(list []Scene) ([]Scene, error) {
	out := make([]Scene, 0, len(list))
	seen := map[string]bool{}
	for _, s := range list {
		s, err := Check(s, st.Recipe)
		if err != nil {
			return nil, err
		}
		for s.ID == "" || seen[s.ID] {
			s.ID = state.NewID()
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := state.SaveJSON(st.dir, file, out); err != nil {
		return nil, err
	}
	st.all = out
	return append(make([]Scene, 0, len(out)), out...), nil
}

// Check is a scene trimmed and refused for what could not run: no name, a kind it does not know, a start scene
// without steps or a step a create route would refuse, a tidy that does nothing.
func Check(s Scene, recipeKnown func(string) bool) (Scene, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return s, errors.New("a scene needs a name")
	}
	if utf8.RuneCountInString(s.Name) > MaxName || !plain(s.Name) {
		return s, fmt.Errorf("scene %q: a name of at most %d characters, no control characters", s.Name, MaxName)
	}
	if s.Kind == "" {
		s.Kind = KindStart
	}
	switch s.Kind {
	case KindStart:
		if s.Tidy != nil {
			return s, fmt.Errorf("scene %q starts things: it tidies nothing", s.Name)
		}
		if len(s.Steps) == 0 || len(s.Steps) > MaxSteps {
			return s, fmt.Errorf("scene %q needs one to %d steps", s.Name, MaxSteps)
		}
		steps := make([]Step, 0, len(s.Steps))
		for i, step := range s.Steps {
			step, err := CheckStep(step, recipeKnown)
			if err != nil {
				return s, fmt.Errorf("scene %q, step %d: %w", s.Name, i+1, err)
			}
			steps = append(steps, step)
		}
		s.Steps = steps
	case KindTidy:
		if len(s.Steps) > 0 {
			return s, fmt.Errorf("scene %q tidies: it starts nothing", s.Name)
		}
		tidy, err := checkTidy(s.Tidy)
		if err != nil {
			return s, fmt.Errorf("scene %q: %w", s.Name, err)
		}
		s.Tidy = &tidy
	default:
		return s, fmt.Errorf("scene %q: a scene starts things or tidies, not %q", s.Name, s.Kind)
	}
	return s, nil
}

// CheckStep is a step trimmed and refused where a recipe would be (recipes.CheckStart), or where it names no machine,
// no absolute folder, both an agent and a recipe, a recipe the cookbook lacks, a tab name too long, a layout herdr
// could not lay out.
func CheckStep(step Step, recipeKnown func(string) bool) (Step, error) {
	step.Machine, step.Dir, step.Agent, step.Recipe = strings.TrimSpace(step.Machine), strings.TrimSpace(step.Dir), strings.TrimSpace(step.Agent), strings.TrimSpace(step.Recipe)
	step.Model, step.Args, step.Prompt, step.Tab = strings.TrimSpace(step.Model), strings.TrimSpace(step.Args), strings.TrimSpace(step.Prompt), strings.TrimSpace(step.Tab)
	if step.Machine == "" {
		return step, errors.New("a machine is required")
	}
	if !rpath.Absolute(step.Dir) {
		return step, errors.New("an absolute folder is required")
	}
	step.Dir = cleanDir(step.Dir)
	switch {
	case step.Recipe != "":
		if step.Agent != "" || step.Model != "" || step.Args != "" || step.Prompt != "" {
			return step, errors.New("a recipe brings its own agent, model, arguments and prompt")
		}
		if recipeKnown != nil && !recipeKnown(step.Recipe) {
			return step, errors.New("the recipe it names is gone")
		}
	case step.Agent == "":
		return step, errors.New("an agent, a recipe or a plain shell is required")
	default:
		if err := recipes.CheckStart(step.Agent, step.Model, step.Args, step.Prompt); err != nil {
			return step, err
		}
	}
	if utf8.RuneCountInString(step.Tab) > MaxTab || !plain(step.Tab) {
		return step, fmt.Errorf("a tab name of at most %d characters, no control characters", MaxTab)
	}
	if step.Layout != nil {
		if err := checkLayout(*step.Layout); err != nil {
			return step, err
		}
	}
	return step, nil
}

// checkTidy is the tidy's choices with their numbers in range, the machines once each; at least one choice is on.
func checkTidy(t *Tidy) (Tidy, error) {
	if t == nil || !(t.Shells || t.Idle || t.Compact || t.Worktrees) {
		return Tidy{}, errors.New("a tidy needs at least one thing to do")
	}
	out := *t
	if !out.Idle {
		out.IdleHours = 0
	} else if out.IdleHours == 0 {
		out.IdleHours = DefaultIdleHours
	}
	if out.Idle && (out.IdleHours < 1 || out.IdleHours > MaxIdleHours) {
		return Tidy{}, fmt.Errorf("agents idle for 1 to %d hours", MaxIdleHours)
	}
	if !out.Compact {
		out.CompactAt = 0
	} else if out.CompactAt == 0 {
		out.CompactAt = DefaultCompactAt
	}
	if out.Compact && (out.CompactAt < 10 || out.CompactAt > 95) {
		return Tidy{}, errors.New("a context share of 10 to 95 %")
	}
	var machines []string
	seen := map[string]bool{}
	for _, id := range t.Machines {
		if id = strings.TrimSpace(id); id != "" && !seen[id] {
			seen[id] = true
			machines = append(machines, id)
		}
	}
	out.Machines = machines
	return out, nil
}

// Covers says whether the tidy reaches a machine: every one when it names none.
func (t Tidy) Covers(machine string) bool {
	if len(t.Machines) == 0 {
		return true
	}
	for _, id := range t.Machines {
		if id == machine {
			return true
		}
	}
	return false
}

// plain says a text holds no control characters.
func plain(text string) bool {
	for _, r := range text {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
