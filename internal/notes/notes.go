// Package notes keeps one free-text note per checkout, a machine and a folder, for whatever the person wants to
// remember about it, after Orca's worktree notes. The notes live in notes.json in the state dir and outlive the
// workspace that showed them: a folder opened again finds its note.
package notes

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/state"
)

const file = "notes.json"

// MaxText is the most a note may hold, in bytes.
const MaxText = 4096

var (
	ErrNoPlace  = errors.New("a note needs a machine and a folder")
	ErrRelative = errors.New("a note's folder must be a full path")
	ErrTooLong  = fmt.Errorf("a note holds %d bytes at most", MaxText)
	ErrNotText  = errors.New("a note holds text, line breaks and tabs only")
)

// Note is one checkout's note and when it was last written.
type Note struct {
	Machine string    `json:"machine"`
	Dir     string    `json:"dir"`
	Text    string    `json:"text"`
	At      time.Time `json:"at"`
}

// Store holds the notes by machine and folder.
type Store struct {
	mu    sync.Mutex
	dir   string
	notes map[string]Note
	now   func() time.Time
}

// Open reads the notes kept in dir; a damaged file is logged once and read as none.
func Open(dir string) *Store {
	s := &Store{dir: dir, notes: map[string]Note{}, now: time.Now}
	var stored []Note
	if err := state.LoadJSON(dir, file, &stored); err != nil {
		log.Printf("notes: %s unreadable, starting empty: %v", file, err)
	}
	for _, note := range stored {
		if note.Machine != "" && note.Dir != "" && note.Text != "" {
			s.notes[key(note.Machine, note.Dir)] = note
		}
	}
	return s
}

// Clean is the folder in its machine's own spelling, so herdr's name for it and the page's find the same note.
func Clean(folder string) string {
	folder = strings.TrimSpace(folder)
	return rpath.For(folder).Clean(folder)
}

func key(machine, folder string) string { return machine + "|" + Clean(folder) }

// Of is the folder's note, "" for none.
func (s *Store) Of(machine, folder string) string {
	if s == nil || folder == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notes[key(machine, folder)].Text
}

// Text is a note as it is kept: line breaks as \n, the ends trimmed, refused past MaxText or holding anything but text.
func Text(text string) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	if !utf8.ValidString(text) {
		return "", ErrNotText
	}
	for _, r := range text {
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || r == ' ' || r == ' ') {
			return "", ErrNotText
		}
	}
	if len(text) > MaxText {
		return "", ErrTooLong
	}
	return text, nil
}

// Set keeps the folder's note, or removes it when the text is empty; it answers the text as kept.
func (s *Store) Set(machine, folder, text string) (string, error) {
	if strings.TrimSpace(machine) == "" || strings.TrimSpace(folder) == "" {
		return "", ErrNoPlace
	}
	if !rpath.Absolute(strings.TrimSpace(folder)) {
		return "", ErrRelative
	}
	kept, err := Text(text)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(machine, folder)
	if kept == "" {
		if _, ok := s.notes[k]; !ok {
			return "", nil
		}
		delete(s.notes, k)
		return "", s.saveLocked()
	}
	s.notes[k] = Note{Machine: machine, Dir: Clean(folder), Text: kept, At: s.now().UTC()}
	return kept, s.saveLocked()
}

// Remove drops the folder's note; a folder with none is no error.
func (s *Store) Remove(machine, folder string) error {
	_, err := s.Set(machine, folder, "")
	return err
}

func (s *Store) saveLocked() error {
	if s.dir == "" {
		return nil
	}
	list := make([]Note, 0, len(s.notes))
	for _, note := range s.notes {
		list = append(list, note)
	}
	sort.Slice(list, func(i, j int) bool { return key(list[i].Machine, list[i].Dir) < key(list[j].Machine, list[j].Dir) })
	return state.SaveJSON(s.dir, file, list)
}
