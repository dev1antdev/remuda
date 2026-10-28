// Package recipes keeps the saved ways to start an agent — a name, the agent, its model and extra arguments, a first
// prompt, and a folder on a machine when the recipe belongs to one — as one JSON list in the state dir replaced whole.
package recipes

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/state"
)

const file = "recipes.json"

// Recipe is one saved start: what the New workspace dialog's agent and Options hold, named.
type Recipe struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Agent   string `json:"agent"`
	Model   string `json:"model,omitempty"`   // as the agent's own CLI names it, "" for its default
	Args    string `json:"args,omitempty"`    // extra arguments as the person typed them
	Prompt  string `json:"prompt,omitempty"`  // sent once the agent is up
	Dir     string `json:"dir,omitempty"`     // the folder the recipe belongs to, "" for any
	Machine string `json:"machine,omitempty"` // the machine of that folder, only with Dir
}

type Store struct {
	dir string
	mu  sync.Mutex
	all []Recipe
}

func New(dir string) *Store {
	st := &Store{dir: dir}
	if err := state.LoadJSON(dir, file, &st.all); err != nil {
		log.Printf("%s is unreadable (%v): no recipes until it is fixed", file, err)
	}
	if st.all == nil {
		st.all = []Recipe{}
	}
	return st
}

func (st *Store) List() []Recipe {
	st.mu.Lock()
	defer st.mu.Unlock()
	return copyOf(st.all)
}

// copyOf is a copy that is never nil, so the client gets [] rather than null.
func copyOf(list []Recipe) []Recipe {
	return append(make([]Recipe, 0, len(list)), list...)
}

// Replace keeps the whole list as given, each recipe checked and trimmed; ids the client did not send, or sent twice, are made up here.
func (st *Store) Replace(list []Recipe) ([]Recipe, error) {
	out := make([]Recipe, 0, len(list))
	seen := map[string]bool{}
	for _, r := range list {
		r, err := Check(r)
		if err != nil {
			return nil, err
		}
		for r.ID == "" || seen[r.ID] {
			r.ID = state.NewID()
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := state.SaveJSON(st.dir, file, out); err != nil {
		return nil, err
	}
	st.all = out
	return copyOf(out), nil
}

// Check is a recipe trimmed and refused for what the create routes would refuse: no name, an agent herdr does not
// know, a model for a kind without a model flag, arguments a shell could not split, extras on a plain shell, a machine without a folder.
func Check(r Recipe) (Recipe, error) {
	r.Name, r.Agent, r.Model = strings.TrimSpace(r.Name), strings.TrimSpace(r.Agent), strings.TrimSpace(r.Model)
	r.Args, r.Prompt, r.Dir, r.Machine = strings.TrimSpace(r.Args), strings.TrimSpace(r.Prompt), strings.TrimSpace(r.Dir), strings.TrimSpace(r.Machine)
	if r.Name == "" {
		return r, errors.New("a recipe needs a name")
	}
	if r.Agent == "" {
		return r, fmt.Errorf("recipe %q names no agent", r.Name)
	}
	if !sessions.KnownAgent(r.Agent) {
		return r, fmt.Errorf("recipe %q: %q is not an agent herdr knows", r.Name, r.Agent)
	}
	if r.Agent == "shell" {
		if r.Model != "" || r.Args != "" || r.Prompt != "" {
			return r, fmt.Errorf("recipe %q: a plain shell takes no model, arguments or prompt", r.Name)
		}
	} else {
		args, err := sessions.SplitArgs(r.Args)
		if err != nil {
			return r, fmt.Errorf("recipe %q: %w", r.Name, err)
		}
		if _, err := sessions.StartArgs(r.Agent, sessions.Start{Model: r.Model, Args: args}); err != nil {
			return r, fmt.Errorf("recipe %q: %w", r.Name, err)
		}
	}
	if r.Machine != "" && r.Dir == "" {
		return r, fmt.Errorf("recipe %q names a machine but no folder", r.Name)
	}
	return r, nil
}
