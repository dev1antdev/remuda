package worktrees

// A new worktree's setup script, after Orca's worktree hooks: the repository's own (herdr.yaml, else orca.yaml) then the
// person's local one, run in their login shell with the worktree's places in HERDR_* (and Orca's and Conductor's names
// for them), in the new worktree's own terminal — before its agent, or in a Setup tab beside it — ending on a line that
// says how it went. The script is written to a file in the machine's temp folder, which removes itself as it starts,
// and the shell is handed one short line to run it.

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// The lines a setup ends on.
const (
	SetupFinished = "Setup finished."
	setupFailed   = "Setup failed (exit "
)

// SetupWait is how long the agent waits for a setup run before it; SetupPoll how often the pane is read meanwhile.
var (
	SetupWait = 30 * time.Minute
	SetupPoll = 2 * time.Second
)

// SetupEnv is the worktree's places as the script sees them: herdr's names, and Orca's and Conductor's for the same.
func SetupEnv(root, worktree string) [][2]string {
	name := rpath.For(worktree).Base(worktree)
	return [][2]string{
		{"HERDR_ROOT_PATH", root}, {"HERDR_WORKTREE_PATH", worktree}, {"HERDR_WORKSPACE_NAME", name},
		{"ORCA_ROOT_PATH", root}, {"ORCA_WORKTREE_PATH", worktree}, {"ORCA_WORKSPACE_NAME", name},
		{"CONDUCTOR_ROOT_PATH", root},
	}
}

// SetupScript is the team's script then the local one, as one shell script.
func SetupScript(team, local string) string {
	var parts []string
	for _, s := range []string{team, local} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

// SetupFile is the file a setup runs as: it removes itself, enters the worktree, runs the script in the person's login
// shell (an interactive bash, zsh or ksh, so what their profile puts on PATH is there; sh for any other) and says how it went.
func SetupFile(body, root, worktree string) string {
	var b strings.Builder
	b.WriteString("rm -f \"$0\"\n")
	var names []string
	for _, kv := range SetupEnv(root, worktree) {
		b.WriteString(kv[0] + "=" + script.Quote(kv[1]) + "\n")
		names = append(names, kv[0])
	}
	b.WriteString("export " + strings.Join(names, " ") + "\n")
	b.WriteString("cd \"$HERDR_WORKTREE_PATH\" || { echo 'Setup failed (exit 1).'; exit 1; }\n")
	b.WriteString("setup=" + script.Quote(body) + "\n")
	b.WriteString("case \"${SHELL##*/}\" in\n  bash|zsh|ksh|mksh) \"$SHELL\" -lic \"$setup\" ;;\n  *) sh -c \"$setup\" ;;\nesac\n")
	b.WriteString("rc=$?\nif [ \"$rc\" -eq 0 ]; then echo '" + SetupFinished + "'; else echo \"" + setupFailed + "$rc).\"; fi\n")
	return b.String()
}

// writeScript writes the file into the machine's temp folder, readable by the person alone, and prints where.
func writeScript(file string) string {
	return "f=$(mktemp \"${TMPDIR:-/tmp}/remuda-setup.XXXXXX\") || exit 3\nchmod 600 \"$f\"\nprintf '%s' " + script.Quote(file) + " > \"$f\" || exit 3\nprintf '%s\\n' \"$f\"\n"
}

// WriteSetup puts the setup's file on the machine and answers the line that runs it.
func WriteSetup(host shell.Shell, body, root, worktree string) (string, error) {
	out, err := shell.Text(host, writeScript(SetupFile(body, root, worktree)), Timeout)
	if err != nil {
		return "", errors.New(lastLine(out, err.Error()))
	}
	path := strings.TrimSpace(out)
	if path == "" {
		return "", errors.New("the machine's temp folder took no file")
	}
	return "sh " + script.Quote(path), nil
}

var failedLine = regexp.MustCompile(`(?m)^Setup failed \(exit (\d+)\)\.\s*$`)

// SetupOutcome reads a pane's recent rows for the line a setup ends on: whether it ended, and its exit code.
func SetupOutcome(rows string) (ended bool, code int) {
	for _, line := range strings.Split(rows, "\n") {
		if strings.TrimSpace(line) == SetupFinished {
			return true, 0
		}
	}
	if m := failedLine.FindStringSubmatch(rows); m != nil {
		code, _ = strconv.Atoi(m[1])
		return true, code
	}
	return false, 0
}

// Watched is what waiting for a setup needs of herdr's client.
type Watched interface {
	PaneRecent(paneID string, n int) (string, bool, error)
	PaneAtPrompt(paneID string) (bool, error)
}

// AwaitSetup waits for the setup in the pane to end and its shell to hold the terminal again, at most SetupWait.
func AwaitSetup(pane Watched, paneID string) (int, error) {
	deadline := time.Now().Add(SetupWait)
	ended, code := false, 0
	for time.Now().Before(deadline) {
		time.Sleep(SetupPoll)
		if !ended {
			rows, _, err := pane.PaneRecent(paneID, 400)
			if err != nil {
				return 0, err
			}
			ended, code = SetupOutcome(rows)
			if !ended {
				continue
			}
		}
		if at, err := pane.PaneAtPrompt(paneID); err == nil && at {
			return code, nil
		}
	}
	return 0, errors.New("the setup was still running after " + SetupWait.String())
}
