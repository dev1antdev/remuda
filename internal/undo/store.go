package undo

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

// What the cockpit keeps: the newest KeepPerFolder turns of every folder, and at most MaxTotal of patches in all, the
// oldest going first.
var (
	KeepPerFolder = 10
	MaxTotal      = 200 << 20
)

// Store keeps the turns on the cockpit, under <state>/undo: turns/<id>.json with turns/<id>.start.patch and
// turns/<id>.end.patch beside it, and open/<id>.json with open/<id>.patch for the turns still going, so a restart of
// remuda mid-turn keeps the start. Every file is 0600.
type Store struct {
	dir string

	mu    sync.Mutex
	turns []Turn // newest first
}

// OpenStore loads the turns kept under stateDir; a damaged file is skipped and said once.
func OpenStore(stateDir string) *Store {
	s := &Store{dir: filepath.Join(stateDir, "undo")}
	entries, err := os.ReadDir(s.turnsDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("undo: the turns could not be listed (%v)", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			if tempLeft(name) {
				_ = os.Remove(filepath.Join(s.turnsDir(), name))
			}
			continue
		}
		var turn Turn
		data, err := os.ReadFile(filepath.Join(s.turnsDir(), name))
		if err == nil {
			err = json.Unmarshal(data, &turn)
		}
		if err != nil || turn.ID == "" {
			log.Printf("undo: %s is unreadable (%v), left out", name, err)
			continue
		}
		s.turns = append(s.turns, turn)
	}
	s.sortLocked()
	return s
}

// tempLeft is a file the atomic writer left behind when remuda stopped mid-write: "<name>.123456789".
func tempLeft(name string) bool {
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 || dot == len(name)-1 {
		return false
	}
	for _, r := range name[dot+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Store) turnsDir() string { return filepath.Join(s.dir, "turns") }
func (s *Store) openDir() string  { return filepath.Join(s.dir, "open") }

func (s *Store) sortLocked() {
	sort.SliceStable(s.turns, func(i, j int) bool { return s.turns[i].Ended.After(s.turns[j].Ended) })
}

// NewID names a turn: its start in UTC, then six hex characters.
func NewID(at time.Time) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return at.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// validID guards an id that names a file.
func validID(id string) bool {
	if len(id) < 8 || len(id) > 40 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
			return false
		}
	}
	return true
}

func mkdir(dir string) error { return os.MkdirAll(dir, 0o700) }

// Add keeps a finished turn with its patches, then keeps the store within its bounds.
func (s *Store) Add(turn Turn, startPatch, endPatch string) error {
	if !validID(turn.ID) {
		return errors.New("a turn needs an id")
	}
	if err := mkdir(s.turnsDir()); err != nil {
		return err
	}
	base := filepath.Join(s.turnsDir(), turn.ID)
	for name, patch := range map[string]string{".start.patch": startPatch, ".end.patch": endPatch} {
		if err := state.WriteFile(base+name, []byte(patch)); err != nil {
			return err
		}
	}
	if err := s.writeTurn(turn); err != nil {
		return err
	}
	s.mu.Lock()
	s.turns = append(s.turns, turn)
	s.sortLocked()
	gone := s.pruneLocked()
	s.mu.Unlock()
	for _, id := range gone {
		s.remove(id)
	}
	return nil
}

func (s *Store) writeTurn(turn Turn) error {
	data, err := json.MarshalIndent(turn, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFile(filepath.Join(s.turnsDir(), turn.ID+".json"), data)
}

// pruneLocked drops what is past the bounds, answering the ids whose files go: a folder's turns past KeepPerFolder,
// then the oldest turns until the patches fit MaxTotal.
func (s *Store) pruneLocked() []string {
	var gone []string
	kept := s.turns[:0]
	perFolder := map[string]int{}
	for _, turn := range s.turns {
		folder := turn.Machine + "|" + turn.Dir
		perFolder[folder]++
		if perFolder[folder] > KeepPerFolder {
			gone = append(gone, turn.ID)
			continue
		}
		kept = append(kept, turn)
	}
	s.turns = kept
	total := 0
	for _, turn := range s.turns {
		total += turn.Size
	}
	for total > MaxTotal && len(s.turns) > 0 {
		oldest := s.turns[len(s.turns)-1]
		s.turns = s.turns[:len(s.turns)-1]
		total -= oldest.Size
		gone = append(gone, oldest.ID)
	}
	return gone
}

func (s *Store) remove(id string) {
	base := filepath.Join(s.turnsDir(), id)
	for _, name := range []string{".json", ".start.patch", ".end.patch"} {
		_ = os.Remove(base + name)
	}
}

// List is a folder's turns, newest first.
func (s *Store) List(machineID, dir string) []Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Turn{}
	for _, turn := range s.turns {
		if turn.Machine == machineID && turn.Dir == dir {
			out = append(out, turn)
		}
	}
	return out
}

// Get is the turn with that id.
func (s *Store) Get(id string) (Turn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, turn := range s.turns {
		if turn.ID == id {
			return turn, true
		}
	}
	return Turn{}, false
}

// Patches reads a turn's two patches back.
func (s *Store) Patches(id string) (string, string, error) {
	if !validID(id) {
		return "", "", errors.New("no such turn")
	}
	base := filepath.Join(s.turnsDir(), id)
	start, err := os.ReadFile(base + ".start.patch")
	if err != nil {
		return "", "", err
	}
	end, err := os.ReadFile(base + ".end.patch")
	return string(start), string(end), err
}

// MarkUndone notes how a turn was undone.
func (s *Store) MarkUndone(id string, undone Undone) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.turns {
		if s.turns[i].ID == id {
			s.turns[i].Undone = &undone
			return s.writeTurn(s.turns[i])
		}
	}
	return errors.New("no such turn")
}

// Forget drops a removed machine's turns, its open ones too.
func (s *Store) Forget(machineID string) {
	s.mu.Lock()
	var gone []string
	kept := s.turns[:0]
	for _, turn := range s.turns {
		if turn.Machine == machineID {
			gone = append(gone, turn.ID)
			continue
		}
		kept = append(kept, turn)
	}
	s.turns = kept
	s.mu.Unlock()
	for _, id := range gone {
		s.remove(id)
	}
	for _, open := range s.LoadOpen() {
		if open.Machine == machineID {
			s.DropOpen(open.ID)
		}
	}
}
