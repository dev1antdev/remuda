// Package gitsync is every repository at a glance and its sync with its remote: the checkouts the fleet's workspaces
// stand on and those a folder of the person's holds, each with its branch, its uncommitted count and how it stands
// against its upstream, read in one script per machine; and the three network actions on one of them at the person's
// press — fetch, a fast-forward pull, a push that never forces — with nothing waiting for a login.
package gitsync

import (
	"strings"
	"time"
)

// Timeout bounds one action on a checkout, its read after it included, and one machine's read of all its repositories.
const Timeout = 60 * time.Second

// read is how every git read here starts: no optional lock taken, no fsmonitor started, so a read never touches the index.
const read = `git --no-optional-locks -c core.fsmonitor=false`

// quietNetwork is the environment a fetch, pull or push runs in: saved credentials and ssh's own configuration still
// work, but nothing may stop to ask for a login. Git tries an askpass before GIT_TERMINAL_PROMPT, so an empty
// GIT_ASKPASS goes with it (an empty one is set, so neither core.askPass nor SSH_ASKPASS is tried); ssh gets
// BatchMode on top of whatever command the repository or the person names for it.
const quietNetwork = `GCM_INTERACTIVE=0 GIT_TERMINAL_PROMPT=0 GIT_ASKPASS= SSH_ASKPASS=; export GCM_INTERACTIVE GIT_TERMINAL_PROMPT GIT_ASKPASS SSH_ASKPASS; ` +
	`sshc=$(git config --get core.sshCommand 2>/dev/null); GIT_SSH_COMMAND="${GIT_SSH_COMMAND:-${sshc:-ssh}} -o BatchMode=yes"; export GIT_SSH_COMMAND; `

// lastLines is git's own words at the end of what it printed, without its hints or its progress: what a failure shows.
func lastLines(output string, n int) string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" || strings.HasPrefix(line, "hint:") || progress(line) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return strings.Join(kept, "\n")
}

// progress is one of git's counting lines ("Receiving objects:  45% (9/20)"), which a failure has no use for.
func progress(line string) bool {
	for _, start := range []string{"remote: Counting", "remote: Compressing", "remote: Enumerating", "remote: Total", "Receiving objects", "Resolving deltas", "Counting objects", "Compressing objects", "Writing objects", "Enumerating objects", "Delta compression"} {
		if strings.HasPrefix(line, start) {
			return true
		}
	}
	return false
}
