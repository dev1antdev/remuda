package worktrees

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// homeAt is this machine with a home of the test's own, where ~/.herdr/worktrees is made
type homeAt struct {
	machine shell.Local
	home    string
}

func (h homeAt) ID() string    { return h.machine.ID() }
func (h homeAt) Label() string { return h.machine.Label() }
func (h homeAt) Local() bool   { return true }
func (h homeAt) Home() string  { return h.home }
func (h homeAt) Run(script string, timeout time.Duration) ([]byte, error) {
	return h.machine.Run(script, timeout)
}
func (h homeAt) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	return h.machine.Stream(script, timeout, sink)
}

// madeRepo is a repository with a commit and its linked worktrees under the home's root, each on a branch of its name
func madeRepo(t *testing.T, names ...string) (host homeAt, main string, trees map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	host = homeAt{machine: shell.Local{Name: "local"}, home: t.TempDir()}
	main = filepath.Join(host.home, "code", "app")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, main, "init", "-q", "-b", "main")
	write(t, filepath.Join(main, "a.txt"), "a\n")
	gitIn(t, main, "add", "a.txt")
	gitIn(t, main, "commit", "-qm", "init")
	trees = map[string]string{}
	for _, name := range names {
		tree := filepath.Join(Root(host.home), "app", name)
		gitIn(t, main, "worktree", "add", "-q", "-b", name, tree)
		trees[name] = tree
	}
	return host, main, trees
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, file, body string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func branches(t *testing.T, main string) string {
	return gitIn(t, main, "branch", "--format=%(refname:short)")
}

func TestOnlyAWorktreeUnderTheRootIsMade(t *testing.T) {
	for folder, want := range map[string]bool{
		"/home/me/.herdr/worktrees/app/task-1":            true,
		"/home/me/.herdr/worktrees/app/task-1/src":        true,
		"/home/me/.herdr/worktrees/app":                   false,
		"/home/me/.herdr/worktrees":                       false,
		"/home/me/.herdr/worktrees/../worktrees/app/x/..": false,
		"/home/me/code/app-wt":                            false,
		"":                                                false,
	} {
		if got := Made("/home/me", folder); got != want {
			t.Errorf("Made(%q) = %v, want %v", folder, got, want)
		}
	}
	if Made("", "/.herdr/worktrees/app/x") {
		t.Error("a machine whose home is unknown has no worktree root")
	}
}

func TestTidyTakesACleanMergedWorktreeDownWithItsBranch(t *testing.T) {
	host, main, trees := madeRepo(t, "done")
	got := Tidy(host, trees["done"], nil)
	if !got.Removed || !got.BranchDeleted || got.Kept != "" || got.BranchKept != "" || got.Branch != "done" {
		t.Fatalf("outcome %+v", got)
	}
	if _, err := os.Stat(trees["done"]); !os.IsNotExist(err) {
		t.Fatalf("the folder stayed: %v", err)
	}
	if strings.Contains(branches(t, main), "done") {
		t.Fatal("the merged branch stayed")
	}
}

func TestTidyKeepsTheBranchOfWorkNotMerged(t *testing.T) {
	host, main, trees := madeRepo(t, "work")
	write(t, filepath.Join(trees["work"], "b.txt"), "b\n")
	gitIn(t, trees["work"], "add", "b.txt")
	gitIn(t, trees["work"], "commit", "-qm", "work")
	got := Tidy(host, trees["work"], nil)
	if !got.Removed || got.BranchDeleted || got.BranchKept == "" {
		t.Fatalf("outcome %+v", got)
	}
	if !strings.Contains(branches(t, main), "work") {
		t.Fatal("the branch with its commit went")
	}
	if host, main, trees := madeRepo(t, "squashed"); true {
		write(t, filepath.Join(trees["squashed"], "c.txt"), "c\n")
		gitIn(t, trees["squashed"], "add", "c.txt")
		gitIn(t, trees["squashed"], "commit", "-qm", "squashed")
		landed := []Landed{{Branch: "squashed", Commit: headOf(t, trees["squashed"])}}
		if got := Tidy(host, trees["squashed"], landed); !got.Removed || !got.BranchDeleted {
			t.Fatalf("a branch whose request was merged stayed: %+v", got)
		}
		if strings.Contains(branches(t, main), "squashed") {
			t.Fatal("the branch of a merged request stayed")
		}
	}
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// TestAMergedRequestVouchesOnlyForItsOwnBranchAtItsOwnCommit: another request's merge, or a commit made after it, leaves
// the branch to git's own check, which keeps what is on no other branch.
func TestAMergedRequestVouchesOnlyForItsOwnBranchAtItsOwnCommit(t *testing.T) {
	commit := func(t *testing.T, tree, name string) string {
		t.Helper()
		write(t, filepath.Join(tree, name), "x\n")
		gitIn(t, tree, "add", name)
		gitIn(t, tree, "commit", "-qm", name)
		return headOf(t, tree)
	}
	for name, landedOf := range map[string]func(tree, merged string) []Landed{
		"another branch's request": func(_, merged string) []Landed { return []Landed{{Branch: "other", Commit: merged}} },
		"no commit known":          func(string, string) []Landed { return []Landed{{Branch: "work"}} },
		"nothing known":            func(string, string) []Landed { return []Landed{{}} },
		"a commit after the merge": func(tree, merged string) []Landed {
			commit(t, tree, "after.txt")
			return []Landed{{Branch: "work", Commit: merged}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			host, main, trees := madeRepo(t, "work")
			merged := commit(t, trees["work"], "b.txt")
			got := Tidy(host, trees["work"], landedOf(trees["work"], merged))
			if !got.Removed || got.BranchDeleted || got.BranchKept == "" {
				t.Fatalf("outcome %+v", got)
			}
			if !strings.Contains(branches(t, main), "work") {
				t.Fatal("the branch with its commits went")
			}
		})
	}
}

func TestLandedAtNeedsTheBranchAndItsLastCommit(t *testing.T) {
	landed := []Landed{{Branch: "work", Commit: "abc"}, {Branch: "other", Commit: "def"}}
	for _, c := range []struct {
		branch, tip string
		want        bool
	}{{"work", "abc", true}, {"work", "def", false}, {"other", "abc", false}, {"", "abc", false}, {"work", "", false}} {
		if got := LandedAt(landed, c.branch, c.tip); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

// TestALookNamesTheCommitTheBranchEndsOn: what the removal dialog's offer goes by.
func TestALookNamesTheCommitTheBranchEndsOn(t *testing.T) {
	host, _, trees := madeRepo(t, "work")
	look, err := LookAt(host, trees["work"])
	if err != nil || look.Tip != headOf(t, trees["work"]) {
		t.Fatalf("%+v, %v", look, err)
	}
}

func TestTidyKeepsAWorktreeWithUncommittedChanges(t *testing.T) {
	host, main, trees := madeRepo(t, "dirty")
	write(t, filepath.Join(trees["dirty"], "a.txt"), "edited\n")
	got := Tidy(host, trees["dirty"], nil)
	if got.Removed || !strings.Contains(got.Kept, "uncommitted changes") || !strings.Contains(got.Kept, "a.txt") {
		t.Fatalf("outcome %+v", got)
	}
	if _, err := os.Stat(filepath.Join(trees["dirty"], "a.txt")); err != nil {
		t.Fatal("the edited file went")
	}
	if !strings.Contains(branches(t, main), "dirty") {
		t.Fatal("the branch went")
	}
}

func TestLeftoversAreTheWorktreesNoWorkspaceHasOpen(t *testing.T) {
	host, _, trees := madeRepo(t, "open", "left", "edited")
	write(t, filepath.Join(trees["edited"], "new.txt"), "new\n")
	stray := filepath.Join(Root(host.home), "app", "stray")
	if err := os.MkdirAll(stray, 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := Leftovers(host, []string{trees["open"], filepath.Join(trees["open"], "src")})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, left := range list {
		names = append(names, left.Name)
	}
	if strings.Join(names, ",") != "edited,left,stray" {
		t.Fatalf("leftovers %v", names)
	}
	edited, left, broken := list[0], list[1], list[2]
	if edited.Branch != "edited" || !strings.Contains(edited.Dirty, "new.txt") || edited.Repo != "app" {
		t.Errorf("edited %+v", edited)
	}
	if left.Branch != "left" || left.Dirty != "" || left.Broken != "" || left.KB <= 0 {
		t.Errorf("left %+v", left)
	}
	if broken.Broken == "" || broken.Branch != "" {
		t.Errorf("stray %+v", broken)
	}
	if list, err := Leftovers(homeAt{machine: shell.Local{Name: "local"}, home: t.TempDir()}, nil); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("a machine with no worktrees: %v %v", list, err)
	}
}

func TestALinkUnderTheRootIsNoLeftover(t *testing.T) {
	host, main, _ := madeRepo(t)
	hand := filepath.Join(host.home, "code", "app-hand")
	gitIn(t, main, "worktree", "add", "-q", "-b", "hand", hand)
	if err := os.MkdirAll(filepath.Join(Root(host.home), "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hand, filepath.Join(Root(host.home), "app", "hand")); err != nil {
		t.Fatal(err)
	}
	list, err := Leftovers(host, []string{hand})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("a link to a worktree made elsewhere, open in a workspace, is a leftover: %+v", list)
	}
}

func TestTidyKeepsNewFilesThatGitStatusIsToldToHide(t *testing.T) {
	host, main, trees := madeRepo(t, "t1")
	gitIn(t, main, "config", "status.showUntrackedFiles", "no")
	write(t, filepath.Join(trees["t1"], "new.go"), "package new\n")
	if list, _ := Leftovers(host, nil); len(list) != 1 || list[0].Dirty == "" {
		t.Fatalf("the leftover with a new file reads clean: %+v", list)
	}
	got := Tidy(host, trees["t1"], nil)
	if got.Removed || !strings.Contains(got.Kept, "uncommitted") {
		t.Fatalf("a worktree with a new file was taken down: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(trees["t1"], "new.go")); err != nil {
		t.Fatalf("the new file is gone: %v", err)
	}
}
