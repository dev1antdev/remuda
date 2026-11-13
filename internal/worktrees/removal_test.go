package worktrees

import (
	"os"
	"path/filepath"
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
