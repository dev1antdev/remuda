package worktrees

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestALookSaysWhatARemovalWouldLose(t *testing.T) {
	host, main, trees := madeRepo(t, "clean", "dirty", "ahead")
	write(t, filepath.Join(trees["dirty"], "draft.txt"), "not committed\n")
	write(t, filepath.Join(trees["dirty"], "a.txt"), "changed\n")
	write(t, filepath.Join(trees["ahead"], "b.txt"), "b\n")
	gitIn(t, trees["ahead"], "add", "b.txt")
	gitIn(t, trees["ahead"], "commit", "-qm", "ahead")

	clean, err := LookAt(host, trees["clean"])
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(main)
	if clean.Branch != "clean" || clean.Dirty != 0 || clean.Unmerged != 0 || !clean.Merged || clean.Loses() {
		t.Errorf("the clean worktree looks %+v", clean)
	}
	if r, _ := filepath.EvalSymlinks(clean.Repo); r != real {
		t.Errorf("its repository is %q, not %q", clean.Repo, real)
	}
	dirty, _ := LookAt(host, trees["dirty"])
	if dirty.Dirty != 2 || dirty.First == "" || !dirty.Loses() {
		t.Errorf("the dirty worktree looks %+v", dirty)
	}
	ahead, _ := LookAt(host, trees["ahead"])
	if ahead.Unpushed != 2 || ahead.Unmerged != 1 || ahead.Merged || ahead.Loses() { // no remote: its first commit is on none either
		t.Errorf("the worktree with a commit of its own looks %+v", ahead)
	}
	if dirty.Stamp() == clean.Stamp() {
		t.Error("two different risks stamp the same")
	}
}

func TestARemovalAsShownForcesPastWhatThePersonWasTold(t *testing.T) {
	host, main, trees := madeRepo(t, "dirty", "ahead")
	write(t, filepath.Join(trees["dirty"], "draft.txt"), "not committed\n")
	write(t, filepath.Join(trees["ahead"], "b.txt"), "b\n")
	gitIn(t, trees["ahead"], "add", "b.txt")
	gitIn(t, trees["ahead"], "commit", "-qm", "ahead")

	dirty, _ := LookAt(host, trees["dirty"])
	out := RemoveAsShown(host, dirty, false)
	if !out.Removed || out.BranchDeleted {
		t.Errorf("the dirty worktree's removal answered %+v", out)
	}
	if _, err := os.Stat(trees["dirty"]); !os.IsNotExist(err) {
		t.Error("the dirty worktree's folder stayed")
	}
	if gitOut(t, main, "branch", "--list", "dirty") == "" {
		t.Error("the branch went though it was kept")
	}

	ahead, _ := LookAt(host, trees["ahead"])
	out = RemoveAsShown(host, ahead, true)
	if !out.Removed || !out.BranchDeleted {
		t.Errorf("the removal with its branch answered %+v", out)
	}
	if gitOut(t, main, "branch", "--list", "ahead") != "" {
		t.Error("the branch flipped to Deleted stayed")
	}
}

func TestAWorktreeWithSubmodulesIsRemovedPastGitsRefusal(t *testing.T) {
	host, main, trees := madeRepo(t, "subs")
	lib := filepath.Join(host.home, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, lib, "init", "-q", "-b", "main")
	write(t, filepath.Join(lib, "l.txt"), "l\n")
	gitIn(t, lib, "add", "l.txt")
	gitIn(t, lib, "commit", "-qm", "lib")
	gitIn(t, trees["subs"], "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "lib")
	gitIn(t, trees["subs"], "commit", "-qm", "the lib")
	inside := filepath.Join(trees["subs"], "lib")
	write(t, filepath.Join(inside, "m.txt"), "only here\n")
	gitIn(t, inside, "add", "m.txt")
	gitIn(t, inside, "commit", "-qm", "pushed nowhere")
	look, err := LookAt(host, trees["subs"])
	if err != nil {
		t.Fatal(err)
	}
	if !look.HasSubmodules || len(look.Submodules) != 1 || look.Submodules[0] != "lib" {
		t.Errorf("the worktree with a submodule looks %+v", look)
	}
	if out := RemoveAsShown(host, look, false); !out.Removed {
		t.Errorf("the removal answered %+v", out)
	}
	_ = main
}

// TestWorkSavedAfterTheConfirmationStays: the check runs again in the script that removes, after the workspace closed.
func TestWorkSavedAfterTheConfirmationStays(t *testing.T) {
	host, _, trees := madeRepo(t, "work")
	draft := filepath.Join(trees["work"], "draft.txt")
	write(t, draft, "first\n")
	look, err := LookAt(host, trees["work"])
	if err != nil {
		t.Fatal(err)
	}
	write(t, draft, "saved since\n")
	got := RemoveAsShown(host, look, false)
	if got.Removed || !strings.Contains(got.Kept, "changed since you confirmed") {
		t.Fatalf("outcome %+v", got)
	}
	if text, err := os.ReadFile(draft); err != nil || string(text) != "saved since\n" {
		t.Errorf("the draft reads %q, %v", text, err)
	}
	look, err = LookAt(host, trees["work"])
	if err != nil {
		t.Fatal(err)
	}
	if got := RemoveAsShown(host, look, false); !got.Removed {
		t.Errorf("work unchanged since the look stays: %+v", got)
	}
}

func TestTheStampFollowsUncommittedWorkInASubmodule(t *testing.T) {
	host, _, trees := madeRepo(t, "subs")
	lib := filepath.Join(host.home, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, lib, "init", "-q", "-b", "main")
	write(t, filepath.Join(lib, "l.txt"), "l\n")
	gitIn(t, lib, "add", "l.txt")
	gitIn(t, lib, "commit", "-qm", "lib")
	gitIn(t, trees["subs"], "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "lib")
	gitIn(t, trees["subs"], "commit", "-qm", "the lib")
	stamp := func() string {
		t.Helper()
		look, err := LookAt(host, trees["subs"])
		if err != nil {
			t.Fatal(err)
		}
		return look.Stamp()
	}
	inside := filepath.Join(trees["subs"], "lib")
	for _, name := range []string{"l.txt", "new.txt"} {
		write(t, filepath.Join(inside, name), "first\n")
		seen := stamp()
		write(t, filepath.Join(inside, name), "second\n")
		if stamp() == seen {
			t.Errorf("more work in the submodule's %s reads as what was shown", name)
		}
	}
}

// TestTheStampFollowsWorkInANewRepositoryAndFailsOnWhatItCannotRead: git lists such a repository as one folder.
func TestTheStampFollowsWorkInANewRepositoryAndFailsOnWhatItCannotRead(t *testing.T) {
	host, _, trees := madeRepo(t, "work")
	tree := trees["work"]
	stamp := func() string {
		t.Helper()
		look, err := LookAt(host, tree)
		if err != nil {
			t.Fatal(err)
		}
		return look.Stamp()
	}
	nested := filepath.Join(tree, "experiment")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, nested, "init", "-q", "-b", "main")
	write(t, filepath.Join(nested, "draft.txt"), "first\n")
	gitIn(t, nested, "add", "draft.txt")
	gitIn(t, nested, "commit", "-qm", "first")
	seen := stamp()
	if again := stamp(); again != seen {
		t.Fatalf("nothing changed and the stamp did: %q, %q", seen, again)
	}
	write(t, filepath.Join(nested, "draft.txt"), "second\n")
	if stamp() == seen {
		t.Error("more work in the new repository reads as what was shown")
	}
	link := filepath.Join(tree, "active")
	if err := os.Symlink("one", link); err != nil {
		t.Fatal(err)
	}
	seen = stamp()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other", link); err != nil {
		t.Fatal(err)
	}
	if stamp() == seen {
		t.Error("a new link pointed elsewhere reads as what was shown")
	}
	if os.Geteuid() == 0 {
		return // root reads everything
	}
	tracked := filepath.Join(tree, "a.txt")
	write(t, tracked, "changed\n")
	if err := os.Chmod(tracked, 0); err != nil {
		t.Fatal(err)
	}
	if stamp() == stamp() {
		t.Error("a changed file that cannot be read reads as unchanged")
	}
	if err := os.Chmod(tracked, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "sealed.txt"), []byte("x\n"), 0); err != nil {
		t.Fatal(err)
	}
	if stamp() == stamp() {
		t.Error("a new file that cannot be read reads as unchanged")
	}
}

// TestAScanThatStopsShortNeverAgrees: a submodule git cannot walk hides the ones after it.
func TestAScanThatStopsShortNeverAgrees(t *testing.T) {
	host, _, trees := madeRepo(t, "subs")
	tree := trees["subs"]
	lib := filepath.Join(host.home, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, lib, "init", "-q", "-b", "main")
	write(t, filepath.Join(lib, "l.txt"), "l\n")
	gitIn(t, lib, "add", "l.txt")
	gitIn(t, lib, "commit", "-qm", "lib")
	for _, name := range []string{"a", "z"} {
		gitIn(t, tree, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, name)
	}
	gitIn(t, tree, "commit", "-qm", "the libs")
	gitIn(t, tree, "config", "-f", ".gitmodules", "--remove-section", "submodule.a")
	write(t, filepath.Join(tree, "z", "l.txt"), "draft\n")
	first, err := LookAt(host, tree)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LookAt(host, tree)
	if err != nil {
		t.Fatal(err)
	}
	if first.Stamp() == second.Stamp() {
		t.Error("a scan that stopped short reads as complete")
	}
}

// submoduleIn gives the worktree a submodule "lib" and answers the folder it is checked out in.
func submoduleIn(t *testing.T, host homeAt, tree string) string {
	t.Helper()
	lib := filepath.Join(host.home, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, lib, "init", "-q", "-b", "main")
	write(t, filepath.Join(lib, "l.txt"), "l\n")
	gitIn(t, lib, "add", "l.txt")
	gitIn(t, lib, "commit", "-qm", "lib")
	gitIn(t, tree, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "lib")
	gitIn(t, tree, "commit", "-qm", "the lib")
	return filepath.Join(tree, "lib")
}

func TestAnEditInASubmoduleSetToBeIgnoredIsShown(t *testing.T) {
	host, _, trees := madeRepo(t, "subs")
	inside := submoduleIn(t, host, trees["subs"])
	gitIn(t, trees["subs"], "config", "-f", ".gitmodules", "submodule.lib.ignore", "dirty")
	gitIn(t, trees["subs"], "commit", "-qam", "ignore it")
	write(t, filepath.Join(inside, "l.txt"), "edited\n")
	look, err := LookAt(host, trees["subs"])
	if err != nil {
		t.Fatal(err)
	}
	if look.Dirty == 0 {
		t.Errorf("the edit is not shown: %+v", look)
	}
}

func TestASubmodulesOtherBranchAndStashAreShown(t *testing.T) {
	for _, keep := range []string{"branch", "stash"} {
		t.Run(keep, func(t *testing.T) {
			host, _, trees := madeRepo(t, "subs")
			inside := submoduleIn(t, host, trees["subs"])
			gitIn(t, inside, "switch", "-q", "-c", "keep")
			write(t, filepath.Join(inside, "l.txt"), "work\n")
			if keep == "branch" {
				gitIn(t, inside, "commit", "-qam", "experiment")
				gitIn(t, inside, "checkout", "-q", "--detach", "origin/HEAD")
			} else {
				gitIn(t, inside, "stash", "-q")
			}
			look, err := LookAt(host, trees["subs"])
			if err != nil {
				t.Fatal(err)
			}
			if len(look.Submodules) != 1 || look.Submodules[0] != "lib" {
				t.Errorf("the submodule looks safe: %+v", look)
			}
		})
	}
}

// TestASubmoduleLeftInTheGitFolderIsStillSaid: a branch without .gitmodules does not take its repository away.
func TestASubmoduleLeftInTheGitFolderIsStillSaid(t *testing.T) {
	host, _, trees := madeRepo(t, "subs")
	tree := trees["subs"]
	submoduleIn(t, host, tree)
	before := gitOut(t, tree, "rev-parse", "HEAD~1")
	gitIn(t, tree, "submodule", "--quiet", "deinit", "-f", "lib")
	gitIn(t, tree, "switch", "-q", "--detach", before)
	if _, err := os.Stat(filepath.Join(tree, ".gitmodules")); err == nil {
		t.Fatal("the commit before still names the submodule")
	}
	look, err := LookAt(host, tree)
	if err != nil {
		t.Fatal(err)
	}
	if !look.HasSubmodules {
		t.Errorf("the worktree looks free of submodules: %+v", look)
	}
	without := look
	without.HasSubmodules = false
	if without.Stamp() == look.Stamp() {
		t.Error("a confirmation does not answer for the submodules")
	}
}
