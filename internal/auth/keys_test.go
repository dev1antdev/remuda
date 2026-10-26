package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func openKeys(t *testing.T, dir string) *Keys {
	t.Helper()
	keys, err := OpenKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestKeysMintCheckRevoke(t *testing.T) {
	dir := t.TempDir()
	keys := openKeys(t, dir)
	key, rec, err := keys.Mint(" GOG ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, KeyPrefix) || len(key) != len(KeyPrefix)+48 || !IsKey(key) {
		t.Fatalf("a key reads rk_ and 48 hex characters, not %q", key)
	}
	if rec.Name != "GOG" || rec.Scope != ScopeTasks || rec.Hash == key || rec.Hash == "" {
		t.Fatalf("minted %+v", rec)
	}
	if got, ok := keys.Check(key); !ok || got.ID != rec.ID {
		t.Fatalf("the key just minted is refused: %v %+v", ok, got)
	}
	if _, ok := keys.Check(KeyPrefix + strings.Repeat("0", 48)); ok {
		t.Fatal("a wrong key passed")
	}
	if _, ok := keys.Check(strings.Repeat("test-token-", 4)); ok {
		t.Fatal("the token is not a key")
	}
	if got, ok := keys.ByName("GOG"); !ok || got.ID != rec.ID {
		t.Fatalf("ByName: %v %+v", ok, got)
	}

	again, rec2, err := keys.Mint("GOG")
	if err != nil {
		t.Fatal(err)
	}
	if list := keys.List(); len(list) != 1 || list[0].ID != rec2.ID {
		t.Fatalf("one key per machine: %+v", list)
	}
	if _, ok := keys.Check(key); ok {
		t.Fatal("the replaced key still passes")
	}
	if _, ok := keys.Check(again); !ok {
		t.Fatal("the new key is refused")
	}

	reloaded := openKeys(t, dir)
	if _, ok := reloaded.Check(again); !ok {
		t.Fatal("the key did not survive a reload of the file")
	}
	if !reloaded.Revoke(rec2.ID) || reloaded.Revoke(rec2.ID) {
		t.Fatal("Revoke answers true once")
	}
	if _, ok := reloaded.Check(again); ok {
		t.Fatal("a revoked key still passes")
	}
	if _, ok := openKeys(t, dir).Check(again); ok {
		t.Fatal("the revocation was not saved")
	}
}

func TestKeysNoteUseOnceAMinute(t *testing.T) {
	keys := openKeys(t, t.TempDir())
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	keys.now = func() time.Time { return now }
	key, rec, _ := keys.Mint("Roofr")
	now = now.Add(30 * time.Second)
	if got, _ := keys.Check(key); !got.LastUsed.Equal(rec.LastUsed) {
		t.Fatalf("bumped within the minute: %v", got.LastUsed)
	}
	now = now.Add(31 * time.Second)
	if got, _ := keys.Check(key); !got.LastUsed.Equal(now) {
		t.Fatalf("not bumped after a minute: %v", got.LastUsed)
	}
}

func TestKeyAllowsTasksOnly(t *testing.T) {
	key := Key{Scope: ScopeTasks}
	for _, path := range []string{"/api/tasks", "/api/tasks/12", "/api/tasks/projects", "/api/tasks/12/comments"} {
		if !key.Allows(path) {
			t.Errorf("%s refused", path)
		}
	}
	for _, path := range []string{"/api/roster", "/api/tasks2", "/api/keys", "/ws/pane", "/", "/api/health"} {
		if key.Allows(path) {
			t.Errorf("%s allowed", path)
		}
	}
	if (Key{}).Allows("/api/tasks") {
		t.Error("a key without a scope opens nothing")
	}
}

// withKey asks a path with a bearer, through a guard whose handler names the caller.
func withKey(h http.Handler, path, bearer string) (int, string) {
	req := httptest.NewRequest("GET", path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, strings.TrimSpace(string(body))
}

func TestGuardLetsAKeyReachTasksOnly(t *testing.T) {
	dir := t.TempDir()
	devices, _ := OpenDevices(dir)
	keys := openKeys(t, dir)
	g := NewGuard(testToken, devices, keys)
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "caller="+CallerName(r.Context())+" device="+DeviceID(r.Context()))
	}))
	key, rec, _ := keys.Mint("GOG")

	if code, body := withKey(h, "/api/tasks?view=open", key); code != 200 || body != "caller=GOG device=" {
		t.Fatalf("tasks with the key: %d %q", code, body)
	}
	if code, body := withKey(h, "/api/tasks/3/comments", key); code != 200 || body != "caller=GOG device=" {
		t.Fatalf("a task's comments with the key: %d %q", code, body)
	}
	if code, body := withKey(h, "/api/roster", key); code != 403 || body != "this key is for tasks only" {
		t.Fatalf("the roster with the key: %d %q", code, body)
	}
	if code, _ := withKey(h, "/api/agent-keys", key); code != 403 {
		t.Fatalf("the keys with a key: %d", code)
	}
	if code, _ := withKey(h, "/api/tasks", KeyPrefix+strings.Repeat("f", 48)); code != 401 {
		t.Fatalf("a wrong key: %d", code)
	}
	if code, body := withKey(h, "/api/roster", testToken); code != 200 || body != "caller= device=" {
		t.Fatalf("the token: %d %q", code, body)
	}
	keys.Revoke(rec.ID)
	if code, _ := withKey(h, "/api/tasks", key); code != 401 {
		t.Fatalf("a revoked key: %d", code)
	}

	bare := NewGuard(testToken, devices, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	if code, _ := withKey(bare, "/api/tasks", key); code != 401 {
		t.Fatalf("a guard without keys took one: %d", code)
	}
}

func TestKeysUnderConcurrentUse(t *testing.T) {
	keys := openKeys(t, t.TempDir())
	key, _, _ := keys.Mint("GOG")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				keys.Check(key)
				if i == 0 {
					keys.Mint("Roofr")
				}
				keys.List()
			}
		}(i)
	}
	wg.Wait()
	if _, ok := keys.Check(key); !ok {
		t.Fatal("the GOG key did not survive Roofr's minting")
	}
}
