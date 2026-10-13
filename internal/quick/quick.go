// Package quick keeps the saved prompts offered in the chat composer: a label
// and a text, for every folder or one in particular, as one JSON list in the
// state dir replaced whole.
package quick

import (
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/text"
)

const file = "quick.json"

type Prompt struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Text  string `json:"text"`
	Dir   string `json:"dir,omitempty"` // "" = every folder
}

type Store struct {
	dir string
	mu  sync.Mutex
	all []Prompt
}

func New(dir string) *Store {
	st := &Store{dir: dir}
	if err := state.LoadJSON(dir, file, &st.all); err != nil {
		log.Printf("%s is unreadable (%v): no quick prompts until it is fixed", file, err)
	}
	if st.all == nil {
		st.all = []Prompt{}
	}
	return st
}

func (st *Store) List() []Prompt {
	st.mu.Lock()
	defer st.mu.Unlock()
	return copyOf(st.all)
}

// copyOf is a copy that is never nil, so the client gets [] rather than null.
func copyOf(list []Prompt) []Prompt {
	return append(make([]Prompt, 0, len(list)), list...)
}

// Replace keeps the whole list as given, checked and trimmed; ids the client did not send, or sent twice, are made up here.
func (st *Store) Replace(list []Prompt) ([]Prompt, error) {
	out := make([]Prompt, 0, len(list))
	seen := map[string]bool{}
	for _, p := range list {
		p.Label, p.Text, p.Dir = strings.TrimSpace(p.Label), strings.TrimSpace(p.Text), strings.TrimSpace(p.Dir)
		if p.Text == "" {
			return nil, errors.New("a prompt is empty")
		}
		if p.Label == "" {
			p.Label = text.FirstLine(p.Text, 40)
		}
		for p.ID == "" || seen[p.ID] {
			p.ID = state.NewID()
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := state.SaveJSON(st.dir, file, out); err != nil {
		return nil, err
	}
	st.all = out
	return copyOf(out), nil
}
