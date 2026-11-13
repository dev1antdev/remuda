package worktrees

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// runSetup writes the setup's file through the machine and runs the line it answers, as the pane's shell would.
func runSetup(t *testing.T, body string) (string, error) {
	t.Helper()
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("TMPDIR", tmp)
	t.Setenv("SHELL", "/bin/sh")
	root, tree := filepath.Join(tmp, "app"), filepath.Join(tmp, "trees", "fix")
	for _, dir := range []string{root, tree} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	line, err := WriteSetup(shell.Local{Name: "local"}, body, root, tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "sh '"+tmp+"/remuda-setup.") {
		t.Fatalf("the line to run it is %q", line)
	}
	file := strings.Trim(strings.TrimPrefix(line, "sh "), "'")
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the setup's file is %v, %v", info, err)
	}
	out, err := exec.Command("sh", "-c", line).CombinedOutput()
	if _, gone := os.Stat(file); !errors.Is(gone, os.ErrNotExist) {
		t.Error("the setup's file stayed once it ran")
	}
	return string(out), err
}

func TestASetupRunsInTheWorktreeWithItsPlaces(t *testing.T) {
	out, err := runSetup(t, "pwd\necho \"root=$HERDR_ROOT_PATH name=$HERDR_WORKSPACE_NAME orca=$ORCA_WORKTREE_PATH conductor=$CONDUCTOR_ROOT_PATH\"\necho 'it'\"'\"'s quoted'")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasSuffix(lines[0], "/trees/fix") || !strings.Contains(lines[1], "/app name=fix orca=") || !strings.Contains(lines[1], "/trees/fix conductor=") {
		t.Errorf("the setup ran as %q", out)
	}
	if lines[2] != "it's quoted" || lines[len(lines)-1] != SetupFinished {
		t.Errorf("the setup said %q", out)
	}
	if ended, code := SetupOutcome(out); !ended || code != 0 {
		t.Errorf("the outcome reads %v %d", ended, code)
	}
}

func TestAFailedSetupSaysItsExitCode(t *testing.T) {
	out, _ := runSetup(t, "echo working\nexit 3")
	if !strings.Contains(out, "Setup failed (exit 3).") {
		t.Errorf("the setup said %q", out)
	}
	if ended, code := SetupOutcome("$ sh '/tmp/x'\nworking\nSetup failed (exit 3).\n$ "); !ended || code != 3 {
		t.Errorf("the outcome reads %v %d", ended, code)
	}
	if ended, _ := SetupOutcome("$ sh '/tmp/x'\npnpm i\n"); ended {
		t.Error("a setup still going reads as ended")
	}
}

func TestTheTeamsScriptRunsBeforeTheLocalOne(t *testing.T) {
	if got := SetupScript(" pnpm i \n", "\nln -s x y\n"); got != "pnpm i\nln -s x y" {
		t.Errorf("got %q", got)
	}
	if SetupScript("", " ") != "" {
		t.Error("two empty scripts make one")
	}
}

// screens answers a pane's rows and prompt in turn, the way herdr would while a setup runs and after
type screens struct {
	rows   []string
	prompt []bool
	reads  int
}

func (s *screens) PaneRecent(string, int) (string, bool, error) {
	text := s.rows[min(s.reads, len(s.rows)-1)]
	s.reads++
	return text, false, nil
}

func (s *screens) PaneAtPrompt(string) (bool, error) {
	at := s.prompt[0]
	if len(s.prompt) > 1 {
		s.prompt = s.prompt[1:]
	}
	return at, nil
}

func TestTheAgentWaitsForTheSetupAndThenItsPrompt(t *testing.T) {
	SetupPoll = time.Millisecond
	defer func() { SetupPoll = 2 * time.Second }()
	pane := &screens{rows: []string{"$ sh x\n", "$ sh x\ninstalling\n", "$ sh x\nSetup failed (exit 2).\n"}, prompt: []bool{false, true}}
	code, err := AwaitSetup(pane, "w1:p1")
	if err != nil || code != 2 {
		t.Errorf("the wait answered %d, %v", code, err)
	}
	if pane.reads != 3 {
		t.Errorf("the rows were read %d times", pane.reads)
	}
}
