package icons

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestOriginOf(t *testing.T) {
	cases := []struct{ remote, host, path string }{
		{"git@github.com:acme/web.git", "github.com", "acme/web"},
		{"https://github.com/acme/web", "github.com", "acme/web"},
		{"https://token@github.com/acme/web.git/", "github.com", "acme/web"},
		{"ssh://git@gitlab.gog.com:2222/gog-backend/games-db.git", "gitlab.gog.com", "gog-backend/games-db"},
		{"https://gitlab.gog.com/group/sub/project.git", "gitlab.gog.com", "group/sub/project"},
		{"git@GitHub.com:Acme/Web.git", "github.com", "Acme/Web"},
	}
	for _, c := range cases {
		host, path, ok := originOf(c.remote)
		if !ok || host != c.host || path != c.path {
			t.Errorf("originOf(%q) = %q %q %v, want %q %q", c.remote, host, path, ok, c.host, c.path)
		}
	}
	for _, bad := range []string{"", "/srv/git/web.git", "git@host:", "https://github.com/acme"} {
		if _, _, ok := originOf(bad); ok {
			t.Errorf("originOf(%q) read a host and an owner", bad)
		}
	}
}

// githubServing answers /acme.png with a picture and anything else with 404, as GitHub's avatar address does.
func githubServing(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/acme.png" && r.URL.Query().Get("size") == "128" {
			_, _ = w.Write(append(append([]byte{}, pngBytes...), "avatar"...)) // a picture of its own, not one of the project's
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	was := GitHubBase
	GitHubBase = server.URL
	t.Cleanup(func() { GitHubBase = was })
}

func TestCandidatesReadsTheProjectsPicturesAndItsOwnersAvatar(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	githubServing(t)
	dir := t.TempDir()
	write := func(name string, data []byte) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("favicon.ico", icoBytes)
	write("public/favicon.svg", []byte(svgText))
	write("static/favicon.svg", []byte(svgText)) // the same picture again: offered once
	write("docs/logo.png", []byte("not a picture at all"))
	write("src/icon.png", append(append([]byte{}, pngBytes...), make([]byte, MaxImage)...)) // too big
	write("ios/App/Assets.xcassets/AppIcon.appiconset/icon-1024.png", pngBytes)
	write("node_modules/pkg/ic_launcher.png", pngBytes) // never searched
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "assets", "logo.png")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:acme/web.git"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	found, err := Candidates(context.Background(), shell.Local{Name: "local"}, Client(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var from []string
	for _, c := range found.Candidates {
		from = append(from, c.Kind+":"+c.From)
		if _, _, err := ParseImage(c.Data); err != nil {
			t.Errorf("%s is no picture: %v", c.From, err)
		}
	}
	want := []string{"file:favicon.ico", "file:public/favicon.svg", "file:ios/App/Assets.xcassets/AppIcon.appiconset/icon-1024.png", "github:github.com/acme"}
	if strings.Join(from, " ") != strings.Join(want, " ") {
		t.Fatalf("candidates %v, want %v", from, want)
	}
	if len(found.Notes) != 0 || len(found.Looked) == 0 {
		t.Fatalf("notes %v, looked %v", found.Notes, found.Looked)
	}
}

func TestCandidatesInAFolderWithNothing(t *testing.T) {
	found, err := Candidates(context.Background(), shell.Local{Name: "local"}, Client(), t.TempDir())
	if err != nil {
		t.Fatalf("a folder with no pictures and no remote is no error: %v", err)
	}
	if len(found.Candidates) != 0 || len(found.Notes) != 1 || !strings.Contains(found.Notes[0], "no origin remote") {
		t.Fatalf("found %+v", found)
	}
}

func TestCandidatesSaysWhyThereIsNoAvatar(t *testing.T) {
	githubServing(t)
	answer := func(remote string) *shell.Fake {
		return &shell.Fake{Name: "GOG", Remote: true, Answer: func(sh string) (string, error) {
			if strings.Contains(sh, "glab api") {
				return "no-glab\n", nil
			}
			return remote + "\n" + script.RS + "\n", nil
		}}
	}
	for remote, note := range map[string]string{
		"":                                     "no origin remote",
		"git@github.com:nobody/web.git":        "GitHub has no avatar for nobody",
		"git@gitlab.gog.com:gog-backend/x.git": "no glab on GOG",
		"/srv/git/plain.git":                   "names no host and owner",
	} {
		found, err := Candidates(context.Background(), answer(remote), Client(), "/p/x")
		if err != nil {
			t.Fatal(err)
		}
		if len(found.Candidates) != 0 || len(found.Notes) != 1 || !strings.Contains(found.Notes[0], note) {
			t.Errorf("remote %q: candidates %d, notes %v; want the note %q", remote, len(found.Candidates), found.Notes, note)
		}
	}
}

func TestCandidatesReadsTheGitLabAvatarThroughGlab(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(pngBytes)
	var glabScript string
	host := &shell.Fake{Name: "GOG", Remote: true, Answer: func(sh string) (string, error) {
		if strings.Contains(sh, "glab api") {
			glabScript = sh
			return "project" + script.US + "\ngroup" + script.US + encoded + "\n", nil
		}
		return "https://gitlab.gog.com/gog-backend/games-db.git\n" + script.RS + "\n", nil
	}}
	found, err := Candidates(context.Background(), host, Client(), "/Users/me/games-db")
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Candidates) != 1 || found.Candidates[0].Kind != "gitlab" || found.Candidates[0].From != "gitlab.gog.com/gog-backend" {
		t.Fatalf("candidates %+v", found.Candidates)
	}
	for _, want := range []string{"--hostname 'gitlab.gog.com'", "'projects/gog-backend%2Fgames-db/avatar'", "'groups/gog-backend/avatar'"} {
		if !strings.Contains(glabScript, want) {
			t.Errorf("the glab script lacks %s: %s", want, glabScript)
		}
	}
	if scripts := host.Scripts(); !strings.Contains(scripts[0], "cd '/Users/me/games-db'") {
		t.Errorf("the files script does not enter the folder: %s", scripts[0])
	}
}
