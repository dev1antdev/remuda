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

// ScopeTasks is an agent's scope: the task routes, and a read of what the agents are doing.
const ScopeTasks = "tasks"

// ViewerPrefix tells a viewer key — a read-only sign-in for a TV or a colleague — from a tasks key and the token.
const ViewerPrefix = "rv_"

// ScopeViewer is the read-only scope: what the page draws, never a pane's terminal, a submit, a key, the tasks or a change.
const ScopeViewer = "viewer"

// readRoutes is what a tasks key may GET beside the tasks: the roster, a conversation's recap and the standup,
// so an agent sees who is on what before it files or takes a task. Nothing there is written.
var readRoutes = map[string]bool{"/api/roster": true, "/api/recap": true, "/api/standup": true}

// viewerRoutes is what a viewer may GET: the roster, the conversations, the screens, the reports and the event stream
// the page draws from; the pane's terminal socket, every submit and key, the tasks and every change stay out.
var viewerRoutes = map[string]bool{
	"/api/roster": true, "/api/chat": true, "/api/screen": true, "/api/scrollback": true, "/api/recap": true, "/api/standup": true,
	"/api/feed": true, "/api/fs/file": true, "/api/fs/stat": true, "/api/health": true, "/api/push": true, "/api/question": true,
	"/api/usage": true, "/ws/events": true,
}

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

// IsKey says whether a bearer value is a key — an agent's or a viewer's — rather than the token.
func IsKey(bearer string) bool {
	return strings.HasPrefix(bearer, KeyPrefix) || strings.HasPrefix(bearer, ViewerPrefix)
}

// Mint makes a tasks-only key for the machine named, replacing the one that machine had: one key per machine.
// The key itself is answered once and never stored.
func (k *Keys) Mint(name string) (key string, rec Key, err error) {
	return k.mint(KeyPrefix, ScopeTasks, name, true)
}

// MintViewer makes a viewer key under a name of the person's own ("the office TV"); several live side by side.
// The key itself is answered once and never stored.
func (k *Keys) MintViewer(name string) (key string, rec Key, err error) {
	return k.mint(ViewerPrefix, ScopeViewer, name, false)
}

// mint makes a key of one scope; replacing drops the key of the same name and scope first.
func (k *Keys) mint(prefix, scope, name string, replacing bool) (key string, rec Key, err error) {
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return "", Key{}, err
	}
	key = prefix + hex.EncodeToString(secret)
	now := k.now()
	rec = Key{ID: state.NewID(), Hash: hashOf(key), Name: strings.TrimSpace(name), Scope: scope, Created: now, LastUsed: now}
	k.mu.Lock()
	defer k.mu.Unlock()
	for id, had := range k.list {
		if replacing && had.Scope == scope && had.Name == rec.Name {
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

// Has says whether a key with this id still stands: a viewer's device is only as good as the key it was made with.
func (k *Keys) Has(id string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	_, ok := k.list[id]
	return ok
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
// and the read routes with GET, a viewer key the viewer routes with GET, nothing else.
func (key Key) Allows(method, path string) bool {
	switch key.Scope {
	case ScopeTasks:
		if path == "/api/tasks" || strings.HasPrefix(path, "/api/tasks/") {
			return true
		}
		return method == http.MethodGet && readRoutes[path]
	case ScopeViewer:
		return ViewerAllows(method, path)
	}
	return false
}

// ViewerAllows is what a viewer — by key, or by the device a viewer link signed in — may ask for: a GET of the viewer
// routes, or of the page itself and its files (everything outside /api, /ws and the code-server proxy under /code).
func ViewerAllows(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	if viewerRoutes[path] {
		return true
	}
	return !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/ws/") && !strings.HasPrefix(path, "/code/")
}

// Reach is what a key of this scope opens, in words, for the log and the refusal.
func (key Key) Reach() string {
	if key.Scope == ScopeViewer {
		return "this link is for viewing only"
	}
	return "this key is for tasks and reading only"
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
