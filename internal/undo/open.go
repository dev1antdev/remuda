package undo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

// Open is a turn still going: where and since when, and the checkpoint taken at its start (its patch on the disk).
type Open struct {
	ID      string    `json:"id"`
	Machine string    `json:"machine"`
	Pane    string    `json:"pane"`
	Dir     string    `json:"dir"`
	Agent   string    `json:"agent,omitempty"`
	Started time.Time `json:"started"`
	Shared  bool      `json:"shared,omitempty"`
	Start   Read      `json:"start"`
}

// SaveOpen keeps a turn's start on the disk while it goes on.
func (s *Store) SaveOpen(open Open, patch string) error {
	if !validID(open.ID) {
		return errors.New("a turn needs an id")
	}
	if err := mkdir(s.openDir()); err != nil {
		return err
	}
	base := filepath.Join(s.openDir(), open.ID)
	if err := state.WriteFile(base+".patch", []byte(patch)); err != nil {
		return err
	}
	data, err := json.Marshal(open)
	if err != nil {
		return err
	}
	return state.WriteFile(base+".json", data)
}

// LoadOpen lists the turns that were going when remuda last stopped.
func (s *Store) LoadOpen() []Open {
	entries, _ := os.ReadDir(s.openDir())
	var out []Open
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var open Open
		data, err := os.ReadFile(filepath.Join(s.openDir(), name))
		if err == nil && json.Unmarshal(data, &open) == nil && validID(open.ID) {
			out = append(out, open)
		}
	}
	return out
}

// OpenPatch reads a going turn's start patch back.
func (s *Store) OpenPatch(id string) (string, error) {
	if !validID(id) {
		return "", errors.New("no such turn")
	}
	data, err := os.ReadFile(filepath.Join(s.openDir(), id+".patch"))
	return string(data), err
}

// DropOpen forgets a going turn's start.
func (s *Store) DropOpen(id string) {
	if !validID(id) {
		return
	}
	base := filepath.Join(s.openDir(), id)
	_ = os.Remove(base + ".json")
	_ = os.Remove(base + ".patch")
}
