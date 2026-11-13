package gitsync

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// The actions on a checkout.
const (
	ActFetch = "fetch"
	ActPull  = "pull"
	ActPush  = "push"
)

// What an action came to.
const (
	Done    = "done"    // git did it
	Refused = "refused" // remuda did not run it, Reason says why
	Failed  = "failed"  // git refused or failed, Output is its words
	Held    = "held"    // a push the secret check holds back until the person says Push anyway
)

// Why an action was refused, or what a done one came to beyond the plain case.
const (
	ReasonAway         = "away"           // the machine cannot be reached now
	ReasonMissing      = "missing"        // the folder is not there
	ReasonNotRepo      = "not-repository" // the folder is no git checkout
	ReasonRunning      = "running"        // another action on the checkout is under way
	ReasonDetached     = "detached"       // HEAD is on no branch
	ReasonNoRemote     = "no-remote"      // the repository has no remote to reach
	ReasonWhichRemote  = "which-remote"   // a branch with no upstream, several remotes and no origin
	ReasonNoUpstream   = "no-upstream"    // nothing to pull from: the branch tracks nothing
	ReasonLocal        = "local-upstream" // the branch tracks a local branch, no remote
	ReasonGone         = "gone"           // the upstream is set but no longer there
	ReasonDiverged     = "diverged"       // both sides have commits: a fast-forward cannot do it
	ReasonBusy         = "busy"           // an agent works, asks or ends a turn in the checkout
	ReasonUpToDate     = "up-to-date"     // a pull with nothing to bring
	ReasonNewBranch    = "new-branch"     // a push that set the branch's upstream
	ReasonCheckFailed  = "check-failed"   // a push the secret check could not clear
	ReasonSecretsFound = "secrets"        // a push the secret check found something in
)

// Outcome is what one action came to, in fields the page puts in its own words.
type Outcome struct {
	Action   string             `json:"action"`
	State    string             `json:"state"`
	Reason   string             `json:"reason,omitempty"`
	Agent    string             `json:"agent,omitempty"` // the kind of agent a pull was refused for
	Doing    string             `json:"doing,omitempty"` // what it is doing there: DoingWork, DoingAsk, DoingEnding
	Remote   string             `json:"remote,omitempty"`
	Branch   string             `json:"branch,omitempty"`
	Upstream string             `json:"upstream,omitempty"`
	Ahead    int                `json:"ahead,omitempty"`
	Behind   int                `json:"behind,omitempty"`
	Count    int                `json:"count,omitempty"`  // the commits a pull brought or a push sent
	Output   string             `json:"output,omitempty"` // git's last lines when it failed, or why the machine is away
	Scan     *review.SecretScan `json:"scan,omitempty"`   // what the secret check found, for a held push
	Repo     *Repo              `json:"repo,omitempty"`   // the checkout read again after
}

func refused(action, reason string) Outcome {
	return Outcome{Action: action, State: Refused, Reason: reason}
}

// fetchScript fetches the remote, pruning the remote-tracking branches it no longer has.
func fetchScript(remote string) string {
	return `git fetch --prune -- ` + script.Quote(remote) + ` </dev/null 2>&1; rc=$?; `
}

// pullScript fetches, then fast-forwards the branch to its upstream only when that is all it takes: a branch with
// commits of its own beside the upstream's is said diverged and nothing changes; git's own refusal of local changes
// in the way stands as it is. Never a merge commit, a rebase or a stash.
func pullScript(remote string) string {
	return fetchScript(remote) + `if [ "$rc" -eq 0 ]; then c=$(` + read + ` rev-list --left-right --count 'HEAD...@{u}' 2>/dev/null); ` +
		`if [ -z "$c" ]; then printf '\036gone\n'; else set -- $c; ` +
		`if [ "$2" = 0 ]; then printf '\036uptodate\n'; elif [ "$1" != 0 ]; then printf '\036diverged %s %s\n' "$1" "$2"; ` +
		`else printf '\036pulled %s\n' "$2"; git -c merge.autoStash=false merge --ff-only --no-edit '@{u}' </dev/null 2>&1; rc=$?; fi; fi; fi; `
}

// pushScript sends the branch to the remote's branch it names; never a force, never a tag (push.followTags off), its
// upstream set when it had none.
func pushScript(remote, branch, target string, setUpstream bool) string {
	flag := ""
	if setUpstream {
		flag = "--set-upstream "
	}
	return `git -c push.followTags=false push ` + flag + `-- ` + script.Quote(remote) + ` ` + script.Quote("refs/heads/"+branch+":"+target) + ` </dev/null 2>&1; rc=$?; `
}

// run runs an action's git part in the checkout without a prompt, then reads the checkout again; the read follows a
// \034 on the way out.
func run(host shell.Host, style rpath.Style, top, action string) (output string, after *Repo, err error) {
	whole := `cd ` + script.Quote(style.Slash(top)) + ` || exit 3; ` + quietNetwork + action +
		`printf '\034'; ( ` + ReadScript(style, []string{top}, nil) + ` ); exit "$rc"`
	answer, runErr := shell.Text(host, whole, Timeout)
	output, readBack, _ := strings.Cut(answer, "\x1c")
	if repos := ParseRead(style, host.Home(), readBack).Repos; len(repos) > 0 {
		after = &repos[0]
	}
	var exit *shell.ExitError
	if runErr != nil && !errors.As(runErr, &exit) {
		return output, after, runErr // a timeout, or the machine went away mid-way
	}
	if exit != nil {
		return output, after, errors.New("exit " + strconv.Itoa(exit.Code))
	}
	return output, after, nil
}

// marker is the line the pull's script says where it went, after \036: "uptodate", "diverged 2 3", "pulled 4".
func marker(output string) (word string, numbers []int) {
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "\x1e") {
			continue
		}
		fields := strings.Fields(line[1:])
		if len(fields) == 0 {
			continue
		}
		for _, field := range fields[1:] {
			n, _ := strconv.Atoi(field)
			numbers = append(numbers, n)
		}
		return fields[0], numbers
	}
	return "", nil
}

// gitWords is what git printed, the markers left out: the last lines a failure shows.
func gitWords(output string) string {
	var kept []string
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "\x1e") {
			kept = append(kept, line)
		}
	}
	return lastLines(strings.Join(kept, "\n"), 6)
}
