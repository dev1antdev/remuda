package worktrees

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/shell"
)

var source = repos.Source{
	Root: "/Users/me/code/app", Name: "app", GitHub: "acme/storefront", Base: "origin/main",
	Local:     []repos.Branch{{Name: "main"}, {Name: "fix"}, {Name: "local-one"}},
	Remote:    []repos.Branch{{Name: "main"}, {Name: "only-on-origin"}},
	Checkouts: []repos.Checkout{{Path: "/Users/me/code/app", Branch: "main", Main: true}, {Path: "/Users/me/.herdr/worktrees/app/fix", Branch: "fix"}},
}

func TestAPlanSaysWhatCreateWillDo(t *testing.T) {
	cases := []struct {
		name, branch, base string
		pick               Pick
		want               Plan
	}{
		{"a branch checked out already", "fix", "", Pick{Kind: "branch", Branch: "fix"}, Plan{Action: "open", Branch: "fix", Path: "/Users/me/.herdr/worktrees/app/fix"}},
		{"a local branch", "local-one", "", Pick{Kind: "branch", Branch: "local-one"}, Plan{Action: "local", Branch: "local-one"}},
		{"a branch only on origin", "only-on-origin", "", Pick{Kind: "branch", Branch: "only-on-origin"}, Plan{Action: "remote", Branch: "only-on-origin", From: "origin/only-on-origin", Track: true}},
		{"a picked branch renamed", "only-2", "", Pick{Kind: "branch", Branch: "only-on-origin"}, Plan{Action: "new", Branch: "only-2", From: "origin/only-on-origin", Untrack: true}},
		{"a new branch from the base", "dev/x", "", Pick{Kind: "new"}, Plan{Action: "new", Branch: "dev/x", From: "origin/main", Untrack: true}},
		{"a new branch from a local base", "dev/x", "main", Pick{Kind: "new"}, Plan{Action: "new", Branch: "dev/x", From: "main"}},
		{"an issue's branch", "dev/feat/198", "", Pick{Kind: "issue", Number: 198}, Plan{Action: "new", Branch: "dev/feat/198", From: "origin/main", Untrack: true, IssueRef: "acme/storefront#198"}},
		{"an issue a pull request works on", "dev/feat/198", "", Pick{Kind: "issue", Number: 198, PR: 212, Head: "dev/feat/198"},
			Plan{Action: "remote", Branch: "dev/feat/198", From: "origin/dev/feat/198", Track: true, Fetch: "+refs/heads/dev/feat/198:refs/remotes/origin/dev/feat/198", IssueRef: "acme/storefront#198", PRRef: "acme/storefront#212"}},
		{"a pull request's head", "cart", "", Pick{Kind: "pr", Number: 212, Head: "cart"},
			Plan{Action: "remote", Branch: "cart", From: "origin/cart", Track: true, Fetch: "+refs/heads/cart:refs/remotes/origin/cart", PRRef: "acme/storefront#212"}},
		{"a fork's pull request", "cart", "", Pick{Kind: "pr", Number: 212, Head: "cart", CrossRepo: true}, Plan{Action: "local", Branch: "cart", Fetch: "+refs/pull/212/head:refs/heads/cart", PRRef: "acme/storefront#212"}},
		{"a pull request's head renamed", "cart-2", "", Pick{Kind: "pr", Number: 212, Head: "cart"}, Plan{Action: "new", Branch: "cart-2", From: "origin/cart", Untrack: true, Fetch: "+refs/heads/cart:refs/remotes/origin/cart"}},
	}
	for _, c := range cases {
		got, err := MakePlan(source, c.branch, c.base, c.pick)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
	for _, bad := range []string{"", "has space", "-leading", "a..b", "end/", "x.lock", "a//b", "a:b", "@"} {
		if _, err := MakePlan(source, bad, "", Pick{Kind: "new"}); err == nil {
			t.Errorf("%q was taken as a branch", bad)
		}
	}
}

func TestTheFolderIsNamedAsHerdrNamesIt(t *testing.T) {
	// as a throwaway herdr 0.9.5 named them: dev/feat/198 → dev-feat-198, Weird.Name_x+y → weird-name-x-y
	for branch, want := range map[string]string{"dev/feat/198": "dev-feat-198", "Weird.Name_x+y": "weird-name-x-y", "feature": "feature"} {
		if got := Slug(branch); got != want {
			t.Errorf("%q: got %q, want %q", branch, got, want)
		}
	}
	if got := FolderFor("/Users/me/trees", "dev/feat/198"); got != "/Users/me/trees/dev-feat-198" {
		t.Errorf("a project's location gives %q", got)
	}
	if FolderFor("", "x") != "" {
		t.Error("no location gives a folder of its own")
	}
}

// gitMaker makes worktrees with git where herdr would, so the git around a create runs on a real repository
type gitMaker struct {
	t    *testing.T
	root string
	home string
}

func (g gitMaker) WorktreeCreateAt(_, branch, base, label, path string) (*herdr.WorktreeCreateResult, error) {
	if path == "" {
		path = filepath.Join(Root(g.home), "app", Slug(branch))
	}
	args := []string{"worktree", "add", "-q", path, branch}
	if base != "" {
		args = []string{"worktree", "add", "-q", "-b", branch, path, base}
	}
	gitIn(g.t, g.root, args...)
	res := &herdr.WorktreeCreateResult{}
	res.Worktree.Path, res.Worktree.Branch = path, branch
	return res, nil
}

func (g gitMaker) WorktreeOpen(_, path, _ string) (*herdr.WorktreeCreateResult, error) {
	res := &herdr.WorktreeCreateResult{}
	res.Worktree.Path = path
	return res, nil
}

func TestACreateSetsTheUpstreamAndKeepsTheLinks(t *testing.T) {
	host, main, _ := madeRepo(t)
	origin := filepath.Join(host.home, "origin.git")
	gitIn(t, host.home, "init", "-q", "--bare", origin)
	gitIn(t, main, "remote", "add", "origin", origin)
	gitIn(t, main, "push", "-q", "origin", "main", "main:cart")
	gitIn(t, main, "fetch", "-q", "origin")
	src, err := repos.Read(host, main)
	if err != nil {
		t.Fatal(err)
	}
	src.GitHub = "acme/storefront" // its origin is a folder; the links are made as for a GitHub one
	maker := gitMaker{t: t, root: main, home: host.home}

	plan, _ := MakePlan(src, "cart", "", Pick{Kind: "pr", Number: 212, Head: "cart"})
	if _, err := Create(host, maker, main, "w1", plan, "", ""); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, main, "rev-parse", "--abbrev-ref", "cart@{upstream}"); got != "origin/cart" {
		t.Errorf("the pull request's branch tracks %q", got)
	}
	if got := gitOut(t, main, "config", "branch.cart.remuda-pr"); got != "acme/storefront#212" {
		t.Errorf("the branch names its pull request as %q", got)
	}

	plan, _ = MakePlan(src, "dev/feat/198", "", Pick{Kind: "issue", Number: 198})
	created, err := Create(host, maker, main, "w1", plan, "", filepath.Join(host.home, "trees", "dev-feat-198"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Worktree.Path != filepath.Join(host.home, "trees", "dev-feat-198") {
		t.Errorf("the worktree went to %q", created.Worktree.Path)
	}
	if out, _ := exec.Command("git", "-C", main, "rev-parse", "--abbrev-ref", "dev/feat/198@{upstream}").CombinedOutput(); !strings.Contains(string(out), "no upstream") {
		t.Errorf("a new branch from origin's tracks something: %s", out)
	}
	if got := gitOut(t, main, "config", "branch.dev/feat/198.remuda-issue"); got != "acme/storefront#198" {
		t.Errorf("the branch names its issue as %q", got)
	}
}

func TestAForksPullRequestIsFetchedAsABranchOfItsOwn(t *testing.T) {
	host, main, _ := madeRepo(t)
	origin := filepath.Join(host.home, "origin.git")
	gitIn(t, host.home, "init", "-q", "--bare", origin)
	gitIn(t, main, "remote", "add", "origin", origin)
	gitIn(t, main, "push", "-q", "origin", "main", "main:refs/pull/7/head")
	src, err := repos.Read(host, main)
	if err != nil {
		t.Fatal(err)
	}
	plan, _ := MakePlan(src, "their-fix", "", Pick{Kind: "pr", Number: 7, Head: "their-fix", CrossRepo: true})
	if _, err := Create(host, gitMaker{t: t, root: main, home: host.home}, main, "w1", plan, "", ""); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, main, "rev-parse", "their-fix"); got != gitOut(t, main, "rev-parse", "main") {
		t.Errorf("the fork's head is %q", got)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

var _ shell.Shell = homeAt{}
