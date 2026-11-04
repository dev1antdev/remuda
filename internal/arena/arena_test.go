package arena

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

func TestCheckRefusesWhatCouldNotStart(t *testing.T) {
	lane := func(agent, model string) LaneRequest {
		return LaneRequest{Machine: "local", Workspace: "w1", Agent: agent, Model: model}
	}
	if err := Check([]LaneRequest{lane("claude", "opus"), lane("codex", "")}); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]LaneRequest{
		"one lane":       {lane("claude", "")},
		"seven lanes":    {lane("claude", ""), lane("claude", ""), lane("claude", ""), lane("claude", ""), lane("claude", ""), lane("claude", ""), lane("claude", "")},
		"a shell":        {lane("claude", ""), lane("shell", "")},
		"an unknown one": {lane("claude", ""), lane("zzz-agent", "")},
		"no workspace":   {lane("claude", ""), {Machine: "local", Agent: "codex"}},
	}
	for name, lanes := range cases {
		if err := Check(lanes); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestTitleOfTakesTheFirstLineCutAtAWord(t *testing.T) {
	if got := TitleOf("  Fix the queue\nand everything else"); got != "Fix the queue" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("word ", 30)
	got := TitleOf(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) > 81 || strings.Contains(got, "wor…") {
		t.Fatalf("got %q", got)
	}
}

func TestStoreKeepsTheArenasAcrossARestart(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, err := st.Create("Fix it", "Fix it", 0, []LaneRequest{{Machine: "local", Workspace: "w1", Agent: "claude"}, {Machine: "gog", Workspace: "w2", Agent: "codex", Model: " gpt-6 "}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 1 || len(a.Lanes) != 2 || a.Lanes[1].Model != "gpt-6" || a.Lanes[0].State != LaneStarting {
		t.Fatalf("arena = %+v", a)
	}
	if err := st.SetLane(a.ID, 2, func(l *Lane) { l.State, l.Pane = LaneRunning, "w9:p1" }); err != nil {
		t.Fatal(err)
	}
	b, _ := st.Create("Second", "Second", 7, []LaneRequest{{Machine: "local", Workspace: "w1", Agent: "claude"}, {Machine: "local", Workspace: "w1", Agent: "gemini"}}, now)

	again := Open(dir)
	list := again.List()
	if len(list) != 2 || list[0].ID != b.ID || list[1].Lanes[1].Pane != "w9:p1" || list[0].TaskID != 7 {
		t.Fatalf("list = %+v", list)
	}
	c, _ := again.Create("Third", "Third", 0, []LaneRequest{{Machine: "local", Workspace: "w1", Agent: "claude"}, {Machine: "local", Workspace: "w1", Agent: "codex"}}, now)
	if c.ID != 3 {
		t.Fatalf("the next id after a restart is %d, want 3", c.ID)
	}
	if err := again.Forget(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Get(a.ID); ok {
		t.Fatal("a forgotten arena is still there")
	}
	if info, err := os.Stat(filepath.Join(dir, fileName)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the file: %v %v", info, err)
	}
}

func TestALateDeliveryMovesOnlyAStartingLane(t *testing.T) {
	st := Open(t.TempDir())
	s := &Service{Store: st}
	a, _ := st.Create("x", "x", 0, []LaneRequest{{Machine: "local", Workspace: "w1", Agent: "claude"}, {Machine: "local", Workspace: "w1", Agent: "codex"}}, time.Now())
	_ = st.SetLane(a.ID, 2, func(l *Lane) { l.State = LaneKept })
	s.settle(a.ID, 1, LaneRunning, "")
	s.settle(a.ID, 2, LaneFailed, "the agent did not take the prompt")
	got, _ := st.Get(a.ID)
	if got.Lane(1).State != LaneRunning || got.Lane(2).State != LaneKept || got.Lane(2).Note != "" {
		t.Fatalf("lanes = %+v", got.Lanes)
	}
	if got.Lane(1).Started != nil {
		t.Fatal("a lane that never made its worktree has a start time")
	}
}

func TestStoreKeepsTheNewestOnly(t *testing.T) {
	st := Open(t.TempDir())
	lanes := []LaneRequest{{Machine: "local", Workspace: "w1", Agent: "claude"}, {Machine: "local", Workspace: "w1", Agent: "codex"}}
	for i := 0; i < Keep+3; i++ {
		if _, err := st.Create("x", "x", 0, lanes, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	list := st.List()
	if len(list) != Keep || list[0].ID != Keep+3 || list[len(list)-1].ID != 4 {
		t.Fatalf("%d arenas, newest %d, oldest %d", len(list), list[0].ID, list[len(list)-1].ID)
	}
}

func TestDamagedFileIsAnEmptyList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := Open(dir)
	if len(st.List()) != 0 {
		t.Fatal("a damaged file listed arenas")
	}
}

func TestParseStatsSumsEachLane(t *testing.T) {
	out := "1\n3\t1\ta.go\n-\t-\tlogo.png\n\x1e\n2\n\x1e\n"
	got := ParseStats(out)
	if got[1] != (Stat{Added: 3, Removed: 1, Files: 2}) || got[2] != (Stat{}) {
		t.Fatalf("got %+v", got)
	}
}

// TestALanesWorkIsSavedWholeAndTheWorktreeGoes runs the scripts on a real repository: a lane that committed, staged,
// edited and added a file is read as numbers, saved as a patch that rebuilds its work on the base, then discarded.
func TestALanesWorkIsSavedWholeAndTheWorktreeGoes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	git := func(dir string, args ...string) string {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(repo, "init", "-q")
	write(filepath.Join(repo, "a.txt"), "one\ntwo\n")
	write(filepath.Join(repo, ".gitignore"), "*.log\n")
	git(repo, "add", "-A")
	git(repo, "commit", "-qm", "init")
	lane := filepath.Join(t.TempDir(), "arena-1-2")
	git(repo, "worktree", "add", "-q", "-b", "arena-1-2", lane)

	host := shell.Local{Name: "local"}
	base, err := shell.Text(host, HeadScript(lane), ScriptTimeout)
	if err != nil {
		t.Fatal(err)
	}
	base = strings.TrimSpace(base)
	write(filepath.Join(lane, "a.txt"), "one\ntwo\nthree\n")
	git(lane, "commit", "-qam", "three")
	write(filepath.Join(lane, "b.txt"), "staged\n")
	git(lane, "add", "b.txt")
	write(filepath.Join(lane, "a.txt"), "ONE\ntwo\nthree\n")
	write(filepath.Join(lane, "c.txt"), "new\nfile\n")
	write(filepath.Join(lane, "debug.log"), "ignored\n")
	indexBefore := git(lane, "diff", "--cached", "--name-only")

	out, err := shell.Text(host, StatsScript([]Lane{{ID: 2, Dir: lane, Base: base}}), ScriptTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseStats(out)[2]; got != (Stat{Added: 5, Removed: 1, Files: 3}) {
		t.Fatalf("stats = %+v from %q", got, out)
	}
	if after := git(lane, "diff", "--cached", "--name-only"); after != indexBefore {
		t.Fatalf("the lane's own index moved: %q → %q", indexBefore, after)
	}

	patch, err := shell.Text(host, PatchScript(lane, base), ScriptTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(patch, "debug.log") {
		t.Fatal("an ignored file went into the patch")
	}
	check := filepath.Join(t.TempDir(), "check")
	git(repo, "worktree", "add", "-q", "--detach", check, base)
	patchFile := filepath.Join(t.TempDir(), "lane.patch")
	write(patchFile, patch)
	git(check, "apply", "--binary", patchFile)
	for name, want := range map[string]string{"a.txt": "ONE\ntwo\nthree\n", "b.txt": "staged\n", "c.txt": "new\nfile\n"} {
		got, _ := os.ReadFile(filepath.Join(check, name))
		if string(got) != want {
			t.Fatalf("%s rebuilt as %q, want %q", name, got, want)
		}
	}

	info, err := worktrees.Inspect(host, lane)
	if err != nil {
		t.Fatal(err)
	}
	if err := worktrees.Discard(host, info); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lane); !os.IsNotExist(err) {
		t.Fatalf("the worktree is still there: %v", err)
	}
	if branches := git(repo, "branch", "--list", "arena-1-2"); branches != "" {
		t.Fatalf("the branch stays: %q", branches)
	}
}
