package arena

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/worktrees"
)

// twoLanes is an arena of two lanes on this machine, none made yet.
func twoLanes(t *testing.T) (*Service, Arena) {
	t.Helper()
	st := Open(t.TempDir())
	a, err := st.Create("x", "x", 0, []LaneRequest{{Machine: "local", Workspace: "w1", Agent: "claude"}, {Machine: "local", Workspace: "w1", Agent: "codex"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Store: st}, a
}

func TestOneLaneIsKeptAndNeverAnotherAfterIt(t *testing.T) {
	s, a := twoLanes(t)
	if _, err := s.Keep(a.ID, 1); err == nil {
		t.Fatal("kept a lane with no worktree")
	}
	_ = s.Store.SetLane(a.ID, 1, func(l *Lane) { l.Dir, l.State = "/tmp/arena-1-1", LaneRunning })
	got, err := s.Keep(a.ID, 1)
	if err != nil || got.Kept != 1 || got.Lane(1).State != LaneKept || got.Lane(2).State != LaneDiscarded {
		t.Fatalf("keep: %v %+v", err, got)
	}
	if _, err := s.Keep(a.ID, 2); err == nil || !strings.Contains(err.Error(), "lane 1 was kept already") {
		t.Fatalf("a second lane was kept after the first: %v", err)
	}
	if again, err := s.Keep(a.ID, 1); err != nil || again.Kept != 1 {
		t.Fatalf("keeping the same lane again, to retry the closes, was refused: %v", err)
	}
}

func TestALaneNotMadeYetIsNeverMadeOnceTheArenaIsDecided(t *testing.T) {
	s, a := twoLanes(t)
	_ = s.Store.SetLane(a.ID, 1, func(l *Lane) { l.Dir, l.State = "/tmp/arena-1-1", LaneRunning })
	if _, err := s.Keep(a.ID, 1); err != nil {
		t.Fatal(err)
	}
	s.startLanes(a) // no machines: a lane it tried to make would panic here
	got, _ := s.Store.Get(a.ID)
	if got.Lane(2).State != LaneDiscarded || got.Lane(2).Dir != "" {
		t.Fatalf("lane 2 = %+v", got.Lane(2))
	}
}

func TestADiscardDeletesTheArenasBranchNotTheOneCheckedOutSince(t *testing.T) {
	info := worktrees.Info{Path: "/w/arena-1-2", Branch: "develop", Repo: "/w/app"}
	if got := ownBranch(info, Lane{Branch: "arena-1-2"}); got.Branch != "arena-1-2" || got.Path != info.Path || got.Repo != info.Repo {
		t.Fatalf("discarded as %+v", got)
	}
	if got := ownBranch(info, Lane{}); got.Branch != "" {
		t.Fatalf("a lane with no branch of its own deletes %q", got.Branch)
	}
}

func TestNoTemporaryFolderMeansNothingIsRemoved(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	head, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(head))
	home := t.TempDir() // its ~/.local/bin is first on the scripts' PATH: an rm there only writes down what it was asked
	bin := filepath.Join(home, ".local", "bin")
	removals := filepath.Join(home, "rm.log")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "rm"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> '"+removals+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(tmp, text string) {
		cmd := exec.Command("sh", "-c", text)
		cmd.Env = append(os.Environ(), "HOME="+home, "TMPDIR="+tmp)
		_, _ = cmd.CombinedOutput()
	}
	for _, text := range []string{StatsScript([]Lane{{ID: 1, Dir: repo, Base: base}}), PatchScript(repo, base)} {
		run(filepath.Join(home, "missing"), text)
	}
	if data, _ := os.ReadFile(removals); len(data) != 0 {
		t.Fatalf("rm ran with no temporary folder made: %q", data)
	}
	tmp := t.TempDir()
	run(tmp, PatchScript(repo, base))
	data, _ := os.ReadFile(removals)
	if lines := strings.Split(strings.TrimSpace(string(data)), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "-rf "+tmp+"/remuda-arena.") {
		t.Fatalf("removed %q, want the one folder made under %s", data, tmp)
	}
}
