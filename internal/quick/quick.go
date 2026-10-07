// Package quick keeps the saved prompts: a label and a text, for every
// folder or one in particular, offered in the chat composer so a phone
// never has to type "run the tests and sum up" again. One JSON list in
// the state dir, replaced whole.
package quick

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dev1antdev/herdr-ui/internal/state"
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
	if b, err := os.ReadFile(filepath.Join(dir, file)); err == nil {
		_ = json.Unmarshal(b, &st.all)
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

// never nil: the client gets [] rather than null
func copyOf(list []Prompt) []Prompt {
	return append(make([]Prompt, 0, len(list)), list...)
}

// Replace keeps the whole list as given, checked and trimmed. IDs the
// client did not send, or sent twice, are made up here.
func (st *Store) Replace(list []Prompt) ([]Prompt, error) {
	out := make([]Prompt, 0, len(list))
	seen := map[string]bool{}
	for _, p := range list {
		p.Label, p.Text, p.Dir = strings.TrimSpace(p.Label), strings.TrimSpace(p.Text), strings.TrimSpace(p.Dir)
		if p.Text == "" {
			return nil, errors.New("a prompt is empty")
		}
		if p.Label == "" {
			p.Label = firstLine(p.Text)
		}
		for p.ID == "" || seen[p.ID] {
			p.ID = newID()
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := state.WriteFile(filepath.Join(st.dir, file), b); err != nil {
		return nil, err
	}
	st.all = out
	return copyOf(out), nil
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if r := []rune(line); len(r) > 40 {
		line = string(r[:40]) + "…"
	}
	return line
}
