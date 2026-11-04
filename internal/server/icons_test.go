package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/icons"
	"github.com/dev1antdev/remuda/internal/review"
)

var onePixel = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// The icon routes: an emoji or a picture set for a folder, the picture served by its hash, the icon cleared; what a
// page can get wrong is refused with a reason.
func TestFolderIconRoutes(t *testing.T) {
	store := icons.New(t.TempDir())
	s := &Server{Icons: store}
	mux := http.NewServeMux()
	s.iconRoutes(mux)

	code, out := taskCall(mux, "PUT", "/api/folder-icons", "", `{"machine":"gog","folder":"/p/games-db","emoji":"🎮"}`)
	if icon, _ := out["icon"].(map[string]any); code != 200 || icon["emoji"] != "🎮" {
		t.Fatalf("emoji: %d %v", code, out)
	}
	code, out = taskCall(mux, "PUT", "/api/folder-icons", "", `{"machine":"gog","folder":"/p/user-profiles","image":"`+onePixel+`"}`)
	icon, _ := out["icon"].(map[string]any)
	hash, _ := icon["image"].(string)
	if code != 200 || len(hash) != 64 {
		t.Fatalf("picture: %d %v", code, out)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/icons/"+hash, nil))
	want, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(onePixel, "data:image/png;base64,"))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != string(want) ||
		!strings.Contains(rec.Header().Get("Cache-Control"), "immutable") || !strings.HasPrefix(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("the picture: %d %v", rec.Code, rec.Header())
	}
	for _, path := range []string{"/api/icons/" + strings.Repeat("0", 64), "/api/icons/not-a-hash"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 404 {
			t.Errorf("GET %s: %d", path, rec.Code)
		}
	}

	refused := map[string]string{
		"no machine":       `{"folder":"/p","emoji":"🎮"}`,
		"relative folder":  `{"machine":"gog","folder":"p","emoji":"🎮"}`,
		"neither":          `{"machine":"gog","folder":"/p"}`,
		"both":             `{"machine":"gog","folder":"/p","emoji":"🎮","image":"` + onePixel + `"}`,
		"letters":          `{"machine":"gog","folder":"/p","emoji":"GOG"}`,
		"not a picture":    `{"machine":"gog","folder":"/p","image":"data:text/html;base64,PGI+"}`,
		"picture of a lie": `{"machine":"gog","folder":"/p","image":"data:image/gif;base64,` + strings.TrimPrefix(onePixel, "data:image/png;base64,") + `"}`,
	}
	for name, body := range refused {
		code, out := taskCall(mux, "PUT", "/api/folder-icons", "", body)
		if msg, _ := out["error"].(string); code != 400 || msg == "" {
			t.Errorf("%s: %d %v, want 400 with a reason", name, code, out)
		}
	}

	if code, _ := taskCall(mux, "DELETE", "/api/folder-icons", "", `{"machine":"gog","folder":"/p/user-profiles"}`); code != 200 {
		t.Fatalf("clear: %d", code)
	}
	if _, ok := store.Of("gog", "/p/user-profiles"); ok {
		t.Fatal("still has an icon after DELETE")
	}
	if _, _, ok := store.Image(hash); ok {
		t.Fatal("the picture outlived its last icon")
	}
}

func TestFaviconRoute(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/i.png"></head></html>`))
			return
		}
		raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(onePixel, "data:image/png;base64,"))
		_, _ = w.Write(raw)
	}))
	defer site.Close()
	s := &Server{Icons: icons.New(t.TempDir())}
	mux := http.NewServeMux()
	s.iconRoutes(mux)

	code, out := taskCall(mux, "POST", "/api/folder-icons/favicon", "", `{"url":"`+site.URL+`"}`)
	if data, _ := out["data"].(string); code != 200 || data != onePixel || out["from"] != site.URL+"/i.png" {
		t.Fatalf("favicon: %d %v", code, out)
	}
	if code, out := taskCall(mux, "POST", "/api/folder-icons/favicon", "", `{"url":"ftp://example.com"}`); code != 400 {
		t.Fatalf("not a web page: %d %v", code, out)
	}
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	if code, out := taskCall(mux, "POST", "/api/folder-icons/favicon", "", `{"url":"`+down.URL+`"}`); code != 502 || out["error"] == nil {
		t.Fatalf("nothing there: %d %v", code, out)
	}
}

// The roster names each workspace's folder — the repository's main checkout for a worktree — and that folder's icon.
func TestRosterCarriesTheFolderAndItsIcon(t *testing.T) {
	store := icons.New(t.TempDir())
	if err := store.SetEmoji("gog", "/p/user-profiles", "👤"); err != nil {
		t.Fatal(err)
	}
	s := &Server{Icons: store, Review: review.New(t.TempDir())}
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{
		{WorkspaceID: "w1", Label: "user-profiles", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/p/user-profiles", RepoKey: "/p/user-profiles/.git", RepoRoot: "/p/user-profiles"}},
		{WorkspaceID: "w2", Label: "small", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/w/small", IsLinkedWorktree: true, RepoKey: "/p/user-profiles/.git"}},
		{WorkspaceID: "w3", Label: "games-db", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/p/games-db"}},
	}}
	sessions := s.sessionsFromSnapshot("gog", snap)
	got := map[string]string{}
	for _, session := range sessions {
		emoji := ""
		if session.Icon != nil {
			emoji = session.Icon.Emoji
		}
		got[session.ID] = session.Folder + " " + emoji
	}
	if got["w1"] != "/p/user-profiles 👤" || got["w2"] != "/p/user-profiles 👤" || got["w3"] != "/p/games-db " {
		t.Fatalf("folders and icons: %v", got)
	}
}

func TestIconCandidatesRouteRefusesWithoutAFolder(t *testing.T) {
	s := &Server{Icons: icons.New(t.TempDir())}
	mux := http.NewServeMux()
	s.iconRoutes(mux)
	for body, want := range map[string]int{
		`{"machine":"local"}`:                     400,
		`{"machine":"local","folder":"relative"}`: 400,
		`{"folder":"/p/web"}`:                     400,
	} {
		if code, out := taskCall(mux, "POST", "/api/folder-icons/candidates", "", body); code != want {
			t.Errorf("%s: %d %v, want %d", body, code, out, want)
		}
	}
}
