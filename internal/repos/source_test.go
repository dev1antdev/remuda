package repos

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repository is a main checkout on main with an origin holding main and a branch only there, a local branch, a linked
// worktree, a team setup script, and origin's address as an https URL with a token in it
func repository(t *testing.T) (root, tree string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	base, _ := filepath.EvalSymlinks(t.TempDir())
	origin, root, tree := filepath.Join(base, "origin.git"), filepath.Join(base, "app"), filepath.Join(base, "trees", "fix")
	gitIn(t, base, "init", "-q", "--bare", origin)
	gitIn(t, base, "init", "-q", "-b", "main", root)
	if err := os.WriteFile(filepath.Join(root, "herdr.yaml"), []byte("scripts:\n  setup: |\n    pnpm i\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-qm", "init")
	gitIn(t, root, "remote", "add", "origin", origin)
	gitIn(t, root, "push", "-q", "origin", "main", "main:only-on-origin")
	gitIn(t, root, "fetch", "-q", "origin")
	gitIn(t, root, "remote", "set-head", "origin", "main")
	gitIn(t, root, "branch", "local-one")
	gitIn(t, root, "worktree", "add", "-q", "-b", "fix", tree)
	gitIn(t, root, "remote", "set-url", "origin", "https://someone:ghp_secret@github.com/acme/storefront.git")
	return root, tree
}

func TestARepositoryIsReadForANewWorktree(t *testing.T) {
	root, tree := repository(t)
	src, err := Read(shell.Local{Name: "local"}, tree) // read from a linked worktree, it answers the main checkout
	if err != nil {
		t.Fatal(err)
	}
	if src.Root != root || src.Name != "app" {
		t.Errorf("the repository is %q named %q", src.Root, src.Name)
	}
	if src.Origin != "https://github.com/acme/storefront.git" || src.GitHub != "acme/storefront" {
		t.Errorf("origin is %q, on GitHub as %q", src.Origin, src.GitHub)
	}
	if src.Base != "origin/main" {
		t.Errorf("a new branch starts from %q", src.Base)
	}
	names := func(list []Branch) map[string]bool {
		out := map[string]bool{}
		for _, b := range list {
			out[b.Name] = true
		}
		return out
	}
	if local := names(src.Local); !local["main"] || !local["local-one"] || !local["fix"] {
		t.Errorf("the local branches are %v", src.Local)
	}
	if remote := names(src.Remote); !remote["main"] || !remote["only-on-origin"] || remote["HEAD"] {
		t.Errorf("origin's branches are %v", src.Remote)
	}
	if len(src.Checkouts) != 2 || !src.Checkouts[0].Main || src.Checkouts[0].Path != root || src.Checkouts[1].Path != tree || src.Checkouts[1].Branch != "fix" {
		t.Errorf("the checkouts are %+v", src.Checkouts)
	}
	if src.Team == nil || src.Team.File != "herdr.yaml" || src.Team.Script != "pnpm i" {
		t.Errorf("the team's script is %+v", src.Team)
	}
}

func TestAFolderOutsideGitSaysSo(t *testing.T) {
	if _, err := Read(shell.Local{Name: "local"}, t.TempDir()); err == nil || err.Error() != "not a git repository" {
		t.Errorf("got %v", err)
	}
}

func TestOnlyAGitHubRemoteNamesARepository(t *testing.T) {
	cases := map[string]string{
		"git@github.com:acme/storefront.git":       "acme/storefront",
		"https://github.com/acme/storefront":       "acme/storefront",
		"ssh://git@github.com/acme/storefront.git": "acme/storefront",
		"https://gitlab.com/acme/storefront.git":   "",
		"git@github.com:acme/team/storefront.git":  "",
		"": "",
	}
	for remote, want := range cases {
		if got := GitHubRepo(remote); got != want {
			t.Errorf("%q: got %q, want %q", remote, got, want)
		}
	}
}

func TestAFetchIsGitsOwnFetch(t *testing.T) {
	root, _ := repository(t)
	gitIn(t, root, "remote", "set-url", "origin", filepath.Join(filepath.Dir(root), "origin.git"))
	gitIn(t, filepath.Dir(root), "--git-dir", filepath.Join(filepath.Dir(root), "origin.git"), "branch", "pushed-later", "main")
	if err := Fetch(shell.Local{Name: "local"}, root); err != nil {
		t.Fatal(err)
	}
	src, err := Read(shell.Local{Name: "local"}, root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range src.Remote {
		found = found || b.Name == "pushed-later"
	}
	if !found {
		t.Errorf("the fetch brought no pushed-later: %v", src.Remote)
	}
}
