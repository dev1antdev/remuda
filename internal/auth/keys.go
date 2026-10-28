package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

const keysFile = "agent-keys.json"

// KeyPrefix tells an agent key from the token in a bearer header.
const KeyPrefix = "rk_"

// ScopeTasks is the one scope a key has today: the task routes, and a read of what the agents are doing.
const ScopeTasks = "tasks"

// readRoutes is what a tasks key may GET beside the tasks: the roster, a conversation's recap and the standup,
// so an agent sees who is on what before it files or takes a task. Nothing there is written.
var readRoutes = map[string]bool{"/api/roster": true, "/api/recap": true, "/api/standup": true}

// Key is one agent key: only the sha256 of the key itself is kept, with the machine it was placed on as its name.
type Key struct {
	ID       string    `json:"id"`
	Hash     string    `json:"hash"`
	Name     string    `json:"name"`
	Scope    string    `json:"scope"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}

// Keys is the store of agent keys, <state dir>/agent-keys.json.
type Keys struct {
	mu   sync.Mutex
	dir  string
	list map[string]*Key
	now  func() time.Time
}

// OpenKeys loads the store; a missing file is an empty store, a damaged one an error.
func OpenKeys(dir string) (*Keys, error) {
	var saved []Key
	if err := state.LoadJSON(dir, keysFile, &saved); err != nil {
		return nil, fmt.Errorf("agent keys: %w", err)
	}
	keys := &Keys{dir: dir, list: map[string]*Key{}, now: time.Now}
	for i := range saved {
		key := saved[i]
		keys.list[key.ID] = &key
	}
	return keys, nil
}

// IsKey says whether a bearer value is an agent key rather than the token.
func IsKey(bearer string) bool { return strings.HasPrefix(bearer, KeyPrefix) }

// Mint makes a tasks-only key for the machine named, replacing the one that machine had: one key per machine.
// The key itself is answered once and never stored.
func (k *Keys) Mint(name string) (key string, rec Key, err error) {
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return "", Key{}, err
	}
	key = KeyPrefix + hex.EncodeToString(secret)
	now := k.now()
	rec = Key{ID: state.NewID(), Hash: hashOf(key), Name: strings.TrimSpace(name), Scope: ScopeTasks, Created: now, LastUsed: now}
	k.mu.Lock()
	defer k.mu.Unlock()
	for id, had := range k.list {
		if had.Name == rec.Name {
			delete(k.list, id)
		}
	}
	k.list[rec.ID] = &rec
	if err := k.saveLocked(); err != nil {
		delete(k.list, rec.ID)
		return "", Key{}, err
	}
	return key, rec, nil
}

// Check finds the key presented, comparing hashes in constant time, and notes it was used, at most once a minute.
func (k *Keys) Check(key string) (Key, bool) {
	if !IsKey(key) {
		return Key{}, false
	}
	presented := hashOf(key)
	k.mu.Lock()
	defer k.mu.Unlock()
	var found *Key
	for _, rec := range k.list {
		if subtle.ConstantTimeCompare([]byte(rec.Hash), []byte(presented)) == 1 {
			found = rec
		}
	}
	if found == nil {
		return Key{}, false
	}
	if now := k.now(); now.Sub(found.LastUsed) >= time.Minute {
		found.LastUsed = now
		if err := k.saveLocked(); err != nil {
			log.Printf("agent keys: %v", err)
		}
	}
	return *found, true
}

// List is every key, the most recently used first.
func (k *Keys) List() []Key {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.sortedLocked()
}

// ByName is the key of the machine named, false when it has none.
func (k *Keys) ByName(name string) (Key, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, rec := range k.list {
		if rec.Name == name {
			return *rec, true
		}
	}
	return Key{}, false
}

// Revoke forgets a key, so it is refused from then on; false for an id the store does not hold.
func (k *Keys) Revoke(id string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.list[id]; !ok {
		return false
	}
	delete(k.list, id)
	if err := k.saveLocked(); err != nil {
		log.Printf("agent keys: %v", err)
	}
	return true
}

// Allows says whether a key of this scope may make a request: a tasks key reaches the task routes with any method
// and the read routes with GET, nothing else.
func (key Key) Allows(method, path string) bool {
	switch key.Scope {
	case ScopeTasks:
		if path == "/api/tasks" || strings.HasPrefix(path, "/api/tasks/") {
			return true
		}
		return method == http.MethodGet && readRoutes[path]
	}
	return false
}

func (k *Keys) saveLocked() error {
	return state.SaveJSON(k.dir, keysFile, k.sortedLocked())
}

func (k *Keys) sortedLocked() []Key {
	out := make([]Key, 0, len(k.list))
	for _, rec := range k.list {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastUsed.Equal(out[j].LastUsed) {
			return out[i].LastUsed.After(out[j].LastUsed)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
