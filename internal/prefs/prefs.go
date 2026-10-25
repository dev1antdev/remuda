// Package prefs keeps the settings a person asked to share between their
// devices: a value and the moment it was set per key, in one JSON file in the
// state dir. The page decides which keys travel; here every key is merged by
// its stamp, the newer one winning, so two devices never fight over a choice.
package prefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

const file = "prefs.json"

// MaxValue bounds one value's JSON, MaxFile the whole file.
const (
	MaxValue = 16 << 10
	MaxFile  = 1 << 20
)

var keyShape = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,80}$`)

// Entry is one shared setting: its JSON value and when it was set.
type Entry struct {
	Value json.RawMessage `json:"value"`
	At    time.Time       `json:"at"`
}

type Store struct {
	dir string
	mu  sync.Mutex
	all map[string]Entry
	now func() time.Time
}

func New(dir string) *Store {
	st := &Store{dir: dir, now: time.Now}
	if err := state.LoadJSON(dir, file, &st.all); err != nil {
		log.Printf("%s is unreadable (%v): no shared settings until it is fixed", file, err)
	}
	if st.all == nil {
		st.all = map[string]Entry{}
	}
	return st
}

// All is every shared setting with its stamp, never nil.
func (st *Store) All() map[string]Entry {
	st.mu.Lock()
	defer st.mu.Unlock()
	return copyOf(st.all)
}

func copyOf(all map[string]Entry) map[string]Entry {
	out := make(map[string]Entry, len(all))
	for k, e := range all {
		out[k] = e
	}
	return out
}

// Put merges the entries given: a key is taken when its stamp is newer than the one kept (equal stamps take the incoming), a null value
// deletes the key, a missing stamp reads as now. The merged whole comes back.
func (st *Store) Put(in map[string]Entry) (map[string]Entry, error) {
	now := st.now()
	for key, e := range in {
		if !keyShape.MatchString(key) {
			return nil, fmt.Errorf("key %q: letters, digits, . _ : - only, up to 80", key)
		}
		if len(e.Value) > MaxValue {
			return nil, fmt.Errorf("key %q: the value is over %d bytes", key, MaxValue)
		}
		if len(e.Value) > 0 && !json.Valid(e.Value) {
			return nil, fmt.Errorf("key %q: the value is not JSON", key)
		}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	merged := copyOf(st.all)
	for key, e := range in {
		if e.At.IsZero() {
			e.At = now
		}
		if kept, ok := merged[key]; ok && kept.At.After(e.At) {
			continue
		}
		if isNull(e.Value) {
			delete(merged, key)
			continue
		}
		merged[key] = e
	}
	b, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b) > MaxFile {
		return nil, fmt.Errorf("the shared settings would be over %d bytes", MaxFile)
	}
	if err := state.SaveJSON(st.dir, file, merged); err != nil {
		return nil, err
	}
	st.all = merged
	return copyOf(merged), nil
}

func isNull(v json.RawMessage) bool {
	return len(v) == 0 || bytes.Equal(bytes.TrimSpace(v), []byte("null"))
}
