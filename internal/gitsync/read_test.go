package gitsync

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestStatusReadsEveryKindOfEntry(t *testing.T) {
	output := strings.Join([]string{
		"# branch.oid 1f2e3d4c5b6a79881f2e3d4c5b6a79881f2e3d4c",
		"# branch.head feature/x",
		"# branch.upstream origin/feature/x",
		"# branch.ab +2 -3",
		"1 .M N... 100644 100644 100644 aaa aaa src/a.go",
		"1 M. N... 100644 100644 100644 aaa bbb src/b.go",
		"2 R. N... 100644 100644 100644 aaa bbb R100 new.go", "old.go",
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc both.go",
		"? notes.txt",
		"! ignored.log",
		"",
	}, "\x00")
	var repo Repo
	repo.Status(output)
	if repo.Branch != "feature/x" || repo.Upstream != "origin/feature/x" || repo.Ahead != 2 || repo.Behind != 3 || repo.Gone || repo.Detached || repo.Initial {
		t.Fatalf("the branch: %+v", repo)
	}
	if repo.Dirty != 5 || repo.Staged != 2 || repo.Conflicts != 1 {
		t.Fatalf("the counts: dirty %d staged %d conflicts %d", repo.Dirty, repo.Staged, repo.Conflicts)
	}

	var gone, detached, initial Repo
	gone.Status("# branch.oid 1f2e\x00# branch.head main\x00# branch.upstream origin/main\x00")
	detached.Status("# branch.oid 1f2e\x00# branch.head (detached)\x00")
	initial.Status("# branch.oid (initial)\x00# branch.head main\x00")
	if !gone.Gone || !detached.Detached || detached.Branch != "" || !initial.Initial {
		t.Fatalf("gone %+v, detached %+v, initial %+v", gone, detached, initial)
	}
}

func TestOneScriptReadsTheWorkspacesAndTheScan(t *testing.T) {
	home := sandbox(t)
	_, clones := remoteAndClones(t, home, 2)
	ahead, behind := clones[0], clones[1]
	commit(t, ahead, "one.txt", "one\n")
	commit(t, behind, "two.txt", "two\n")
	gitIn(t, behind, "push", "-q")
	gitIn(t, behind, "reset", "-q", "--hard", "HEAD~1")
	gitIn(t, behind, "fetch", "-q")
	if err := os.WriteFile(filepath.Join(ahead, "dirty.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(home, "trees", "x")
	gitIn(t, ahead, "worktree", "add", "-q", "-b", "x", worktree)

	projects := filepath.Join(home, "projects")
	alone := filepath.Join(projects, "group", "alone")
	gitIn(t, home, "init", "-q", alone)
	commit(t, alone, "a.txt", "a\n")
	for _, hidden := range []string{filepath.Join(projects, "node_modules", "dep"), filepath.Join(projects, ".cache", "x"), filepath.Join(projects, "a", "b", "c", "d", "too-deep")} {
		gitIn(t, home, "init", "-q", hidden)
	}
	broken := filepath.Join(projects, "broken")
	if err := os.MkdirAll(filepath.Join(broken, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	plain := filepath.Join(home, "plain") // a workspace's folder outside git is no repository
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(home, "linked") // the same checkout spelled through a link reads once
	if err := os.Symlink(ahead, linked); err != nil {
		t.Fatal(err)
	}

	host := shell.Local{Name: "local"}
	style := rpath.For(host.Home())
	out, err := shell.Text(host, ReadScript(style, []string{filepath.Join(ahead, "sub"), ahead, worktree, behind, filepath.Join(home, "no-repo"), plain, linked}, &Scan{Folder: projects, Depth: 3}), Timeout)
	if err != nil {
		t.Fatal(err)
	}
	read := ParseRead(style, home, out)
	byName := map[string]Repo{}
	for _, repo := range read.Repos {
		byName[repo.Name] = repo
	}
	if len(read.Repos) != 4 || read.More || read.ScanMissing {
		t.Fatalf("the repositories: %+v", read)
	}
	if got := byName["clonea"]; !got.Workspace || got.Ahead != 1 || got.Dirty != 1 || !got.Remote || got.Parent != "~/code" || got.Upstream != "origin/main" {
		t.Fatalf("the clone ahead (its worktree folded into it): %+v", got)
	}
	if got := byName["cloneb"]; got.Behind != 1 || got.Fetched.IsZero() {
		t.Fatalf("the clone behind: %+v", got)
	}
	if got := byName["alone"]; got.Workspace || got.Remote || got.Upstream != "" || got.Parent != "~/projects/group" {
		t.Fatalf("the repository the scan found: %+v", got)
	}
	if got := byName["broken"]; got.Error == "" {
		t.Fatalf("a repository git cannot read: %+v", got)
	}
	if read.Repos[0].Name != "clonea" || read.Repos[len(read.Repos)-1].Name != "alone" {
		t.Fatalf("the order: %+v", read.Repos)
	}

	missing, err := shell.Text(host, ReadScript(style, nil, &Scan{Folder: filepath.Join(home, "nowhere"), Depth: 2}), Timeout)
	if err != nil || !ParseRead(style, home, missing).ScanMissing {
		t.Fatalf("a scan of a folder that is not there: %q %v", missing, err)
	}
}

func TestAWindowsMachinesRepositoryIsListedOnce(t *testing.T) {
	record := func(kind, path string) string {
		return "\x1d" + kind + " " + path + "\x1e# branch.head main\x00\x1e0\x1f\x1forigin\x1f\n"
	}
	read := ParseRead(rpath.Style{Windows: true}, `C:\Users\me`, record("w", "/c/Users/me/code/app")+record("s", "C:/Users/me/code/app")+record("s", "C:/Users/me/code/web"))
	if len(read.Repos) != 2 || read.Repos[0].Path != `C:\Users\me\code\app` || !read.Repos[0].Workspace || read.Repos[0].Parent != "~/code" || read.Repos[1].Name != "web" {
		t.Fatalf("%+v", read.Repos)
	}
}

func TestTheScanStopsAtItsCount(t *testing.T) {
	var b strings.Builder
	b.WriteString("\x1dw /home/me/app\x1e# branch.head main\x00\x1e0\x1f\x1forigin\x1f\n")
	for i := range MaxScan + 1 {
		b.WriteString("\x1ds /home/me/code/r" + strconv.Itoa(i) + "\x1e# branch.head main\x00\x1e0\x1f\x1f\x1f\n")
	}
	read := ParseRead(rpath.Style{}, "/home/me", b.String())
	workspaces := 0
	for _, repo := range read.Repos {
		if repo.Workspace {
			workspaces++
		}
	}
	if !read.More || len(read.Repos) != MaxScan+1 || workspaces != 1 {
		t.Fatalf("%d repositories, more %v", len(read.Repos), read.More)
	}
}
