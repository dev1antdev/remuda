package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func byPath(files []File) map[string]File {
	out := map[string]File{}
	for _, file := range files {
		out[file.Path] = file
	}
	return out
}

func TestTheSummarySaysWhatIsStagedAndWhatIsNot(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	write("a.txt", "one\n")
	write("b.txt", "b\n")
	write("gone.txt", "gone\n")
	write("sub/s.txt", "s\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("a.txt", "one\ntwo\n")
	git("add", "a.txt")
	write("a.txt", "one\ntwo\nthree\n")
	write("b.txt", "b2\n")
	git("rm", "-q", "gone.txt")
	write("new.txt", "new\n")
	write("sub/s.txt", "s2\n")

	files := byPath(New(t.TempDir()).Summary(shell.Local{Name: "local"}, dir).Files)
	a := files["a.txt"]
	if a.Index != "M" || a.Work != "M" || a.Staged == nil || a.Staged.Added != 1 || a.Unstaged == nil || a.Unstaged.Added != 1 || a.Added != 2 {
		t.Fatalf("a file partly staged: %+v staged %+v unstaged %+v", a, a.Staged, a.Unstaged)
	}
	if b := files["b.txt"]; b.Index != "" || b.Work != "M" || b.Staged != nil || b.Unstaged == nil {
		t.Fatalf("an edit not staged: %+v", b)
	}
	if gone := files["gone.txt"]; gone.Index != "D" || gone.Staged == nil || gone.Unstaged != nil {
		t.Fatalf("a staged deletion: %+v", gone)
	}
	if fresh := files["new.txt"]; !fresh.Untracked || fresh.Staged != nil {
		t.Fatalf("an untracked file: %+v", fresh)
	}

	sub := byPath(New(t.TempDir()).Summary(shell.Local{Name: "local"}, filepath.Join(dir, "sub")).Files)
	if len(sub) != 1 || sub["s.txt"].Work != "M" {
		t.Fatalf("a folder inside the repository sees its own files by its own names: %+v", sub)
	}
}

func TestTheSummaryNamesAConflict(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	write("c.txt", "base\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("checkout", "-q", "-b", "other")
	write("c.txt", "theirs\n")
	git("commit", "-qam", "theirs")
	git("checkout", "-q", "-")
	write("c.txt", "ours\n")
	git("commit", "-qam", "ours")
	cmd := exec.Command("git", "-c", "user.email=t@t", "-c", "user.name=t", "merge", "-q", "other")
	cmd.Dir = dir
	_ = cmd.Run()

	files := byPath(New(t.TempDir()).Summary(shell.Local{Name: "local"}, dir).Files)
	if c := files["c.txt"]; c.Conflict != "UU" || c.Staged != nil || c.Unstaged != nil {
		t.Fatalf("the conflict: %+v", c)
	}
	read, err := Conflict(shell.Local{Name: "local"}, dir, "c.txt")
	if err != nil || read.Stages != "123" || !strings.Contains(read.Text, "<<<<<<< ") || !strings.Contains(read.Text, ">>>>>>> other") {
		t.Fatalf("the conflict read: %+v %v", read, err)
	}
}

func TestTheSummaryListsTheCommitsNotOnTheRemote(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	origin := t.TempDir()
	exec.Command("git", "init", "-q", "--bare", origin).Run()
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	git("remote", "add", "origin", origin)
	git("push", "-q", "-u", "origin", "main")
	write("a.txt", "a2\n")
	git("commit", "-qam", "Not pushed yet")
	write("a.txt", "a3\n")
	git("commit", "-qam", "Nor this one")

	summary := New(t.TempDir()).Summary(shell.Local{Name: "local"}, dir)
	out := summary.Outgoing
	if out == nil || out.Against != "origin/main" || !out.Upstream || out.Total != 2 || len(out.Commits) != 2 || out.Commits[0].Subject != "Nor this one" || out.Commits[0].Author != "t" {
		t.Fatalf("outgoing: %+v", out)
	}
	other := t.TempDir()
	for _, args := range [][]string{{"clone", "-q", origin, other}, {"-C", other, "-c", "user.email=o@o", "-c", "user.name=o", "commit", "-q", "--allow-empty", "-m", "elsewhere"}, {"-C", other, "push", "-q"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	git("fetch", "-q")
	if out := New(t.TempDir()).Summary(shell.Local{Name: "local"}, dir).Outgoing; out == nil || out.Behind != 1 || out.Total != 2 {
		t.Fatalf("one to pull as of the last fetch: %+v", out)
	}
	git("checkout", "-q", "-b", "feature")
	write("b.txt", "b\n")
	git("add", "-A")
	git("commit", "-qm", "On the feature")
	if out := New(t.TempDir()).Summary(shell.Local{Name: "local"}, dir).Outgoing; out == nil || out.Against != "origin/main" || out.Upstream || out.Total != 3 || out.Behind != 0 {
		t.Fatalf("a branch never pushed compares with origin/main where it left it: %+v", out)
	}

	view, err := ReadCommit(shell.Local{Name: "local"}, dir, git("rev-parse", "HEAD"))
	if err != nil || view.Subject != "On the feature" || len(view.Files) != 1 || view.Files[0].Path != "b.txt" || view.Files[0].Status != "A" || view.Files[0].Added != 1 {
		t.Fatalf("the commit: %+v %v", view, err)
	}
	diff, err := CommitDiff(shell.Local{Name: "local"}, dir, git("rev-parse", "HEAD"), "b.txt")
	if err != nil || !strings.Contains(diff, "+b") {
		t.Fatalf("the commit's diff: %q %v", diff, err)
	}
}

func TestTheBaseIsTheBranchTheWorkWasCutFrom(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	git("checkout", "-q", "-b", "feature")
	write("f.txt", "f\n")
	git("add", "-A")
	git("commit", "-qm", "feature work")
	git("checkout", "-q", "-b", "stacked")
	write("s.txt", "s\n")
	git("add", "-A")
	git("commit", "-qm", "stacked work")
	write("loose.txt", "loose\n")
	git("checkout", "-q", "main")
	write("m.txt", "m\n")
	git("add", "m.txt")
	git("commit", "-qm", "main moved on")
	git("checkout", "-q", "stacked")

	store := New(t.TempDir())
	view, err := store.Base(shell.Local{Name: "local"}, dir, "")
	if err != nil || view.Base != "feature" || view.Ahead != 1 {
		t.Fatalf("a branch stacked on another compares with it: %+v %v", view, err)
	}
	if files := byPath(view.Files); len(files) != 2 || files["s.txt"].Added != 1 || !files["loose.txt"].Untracked {
		t.Fatalf("files: %+v", view.Files)
	}
	pinned, err := store.Base(shell.Local{Name: "local"}, dir, "main")
	if err != nil || pinned.Base != "main" || pinned.Detected != "feature" || pinned.Ahead != 2 || len(pinned.Files) != 3 {
		t.Fatalf("pinned to main: %+v %v", pinned, err)
	}
	diff, err := BaseDiff(shell.Local{Name: "local"}, dir, pinned.MergeBase, "f.txt")
	if err != nil || !strings.Contains(diff, "+f") {
		t.Fatalf("the base diff: %q %v", diff, err)
	}
	if _, err := store.Base(shell.Local{Name: "local"}, dir, "--upload-pack=x"); err == nil {
		t.Fatal("an option reached git as a branch")
	}
	git("checkout", "-q", "main")
	if trunk, _ := store.Base(shell.Local{Name: "local"}, dir, ""); trunk.Base != "" {
		t.Fatalf("main with no remote has nothing to compare with: %+v", trunk)
	}
}

func TestForkBaseRanksByAncestry(t *testing.T) {
	answer := strings.Join([]string{
		"head\x1fhhhh", "branch\x1ffeature", "remote\x1forigin", "originhead\x1forigin/main",
		"anc\x1ffeature\x1f0", "anc\x1forigin/feature\x1f1", "anc\x1frelease\x1f7",
		"trunk\x1forigin/main\x1fmbmb\x1f3", "trunk\x1fmain\x1fmbmb\x1f3", "ref\x1ffeature", "ref\x1fmain",
	}, "\n")
	if head, base, refs := forkBase(answer); head != "hhhh" || base != "main" || len(refs) != 2 {
		t.Fatalf("the trunk three commits back over a branch seven back, ties to the shorter name: %q %q %v", head, base, refs)
	}
	onMain := "head\x1fhhhh\nbranch\x1fmain\nremote\x1forigin\nanc\x1frelease\x1f2\ntrunk\x1forigin/main\x1fhhhh\x1f0\n"
	if _, base, _ := forkBase(onMain); base != "origin/main" {
		t.Fatalf("a trunk compares with origin's: %q", base)
	}
}
