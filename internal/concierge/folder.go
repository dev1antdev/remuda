package concierge

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Folder is the concierge's folder under remuda's state dir on its machine: Claude Code starts there and reads Memory.
const Folder = "concierge"

// Memory is the CLAUDE.md remuda writes into the concierge's folder: who it is, what it may do, how to answer.
const Memory = `# You are remuda's concierge

The person runs coding agents on their machines through remuda, a cockpit over herdr. You are its concierge: you answer their questions about what those agents are doing and about their task list, and you file tasks for them.

Your only tools are remuda's own, the mcp__remuda__ ones:
- agents_list: every agent on every machine, its state and for how long, the first line of its last answer, its pull request, its cost.
- agent_recap: what one agent's conversation came to. standup: what every agent did in a period.
- tasks_list, task_get, projects_list, labels_list: the task list.
- task_create: a new task. It lands in its project's Proposed column for the person to accept; say so. Look with tasks_list first, so nothing is filed twice.
- task_comment: a note on a task.

You cannot read or change files, run commands, go on the web, move a task to another column, or reach the agents themselves. When the person asks for one of those, say so in a sentence and offer what you can do instead: a task, or the words they could send the agent.

Look before you answer: call the tools rather than guess. Answer briefly, in the language the person writes in. Name an agent by its workspace and machine, as agents_list shows them.
`

// FolderScript makes the concierge's folder — under stateDir when that is the cockpit's own, else under the dir
// remuda keeps on the machine — writes Memory into it, and prints the folder.
func FolderScript(stateDir string) string {
	locate := `case "$(uname -s)" in
  Darwin) d="$HOME/Library/Application Support/remuda" ;;
  *) d="${XDG_CONFIG_HOME:-$HOME/.config}/remuda" ;;
esac`
	if stateDir != "" {
		locate = "d=" + script.Quote(stateDir)
	}
	return "umask 077\n" + locate + `
f="$d/` + Folder + `"
mkdir -p "$f" && printf '%s' ` + script.Quote(Memory) + ` > "$f/CLAUDE.md" && printf '%s\n' "$f"`
}

// Prepare makes the concierge's folder on the machine and answers where it is.
func Prepare(host shell.Shell, stateDir string) (string, error) {
	out, err := shell.Text(host, FolderScript(stateDir), 15*time.Second)
	if err != nil {
		return "", err
	}
	folder := strings.TrimSpace(out)
	if !strings.HasSuffix(folder, "/"+Folder) {
		return "", errors.New("the concierge's folder could not be made there")
	}
	return folder, nil
}

// Prepare makes the concierge's folder on a machine: under this remuda's own state dir on the cockpit's machine, so a
// second instance keeps its own, else under the one remuda keeps there.
func (s *Service) Prepare(host shell.Shell, local bool) (string, error) {
	if local {
		return Prepare(host, s.dir)
	}
	return Prepare(host, "")
}

// RemudaOn is the remuda binary the concierge's MCP server runs on a machine: this very one on the cockpit's own,
// else the one setup puts under the machine's home.
func RemudaOn(local bool, home string) string {
	if local {
		if bin, err := os.Executable(); err == nil {
			return bin
		}
	}
	return home + "/.local/bin/remuda"
}
