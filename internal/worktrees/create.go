package worktrees

// A new worktree as the New worktree dialog asks for it, after Orca's: the branch checked out already is opened, a local
// one checked out, one only on origin made a local branch tracking it, a pull request's head fetched first, anything
// else a new branch from the base. herdr's worktree.create makes the checkout; the git around it — the fetch, the
// upstream, the issue or pull request the branch was made for in git config — runs on the machine at the person's press.

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// FetchTimeout is how long a fetch of a branch before a create may take: a big repository's first fetch takes minutes.
var FetchTimeout = 10 * time.Minute

// Pick is what the dialog's Details were filled from.
type Pick struct {
	Kind      string `json:"kind"`                 // branch | issue | pr | new
	Branch    string `json:"branch,omitempty"`     // a branch row's branch, as git names it locally or on origin
	Number    int    `json:"number,omitempty"`     // an issue's or a pull request's
	Head      string `json:"head,omitempty"`       // a pull request's head branch; for an issue, the head of the PR working on it
	PR        int    `json:"pr,omitempty"`         // for an issue, that PR's number
	CrossRepo bool   `json:"cross_repo,omitempty"` // the pull request's head is on a fork
}

// Plan is what Create will do.
type Plan struct {
	Action   string `json:"action"`            // open | local | remote | new
	Branch   string `json:"branch"`            // the branch the worktree has checked out
	Path     string `json:"path,omitempty"`    // open: the checkout there
	From     string `json:"from,omitempty"`    // new: what the branch starts from
	Fetch    string `json:"fetch,omitempty"`   // a refspec fetched from origin first
	Track    bool   `json:"track,omitempty"`   // the branch tracks origin's of its name
	Untrack  bool   `json:"untrack,omitempty"` // a new branch from origin's tracks nothing: its first push makes its own
	IssueRef string `json:"issue,omitempty"`   // owner/repo#N kept as branch.<b>.remuda-issue
	PRRef    string `json:"pr_ref,omitempty"`  // owner/repo#N kept as branch.<b>.remuda-pr
}

var badBranch = regexp.MustCompile(`[\s~^:?*\[\\]|\.\.|@\{|//|^[-/.]|[/.]$|\.lock$`)

// BadBranch says git would refuse the name as a branch (check-ref-format's rules, as far as a name typed goes).
func BadBranch(name string) bool {
	if name == "" || name == "@" || badBranch.MatchString(name) {
		return true
	}
	for _, r := range name {
		if r < ' ' || r == 0x7f {
			return true
		}
	}
	return false
}

// MakePlan is what a create of the branch does in the repository as it was read, from what the person picked.
func MakePlan(src repos.Source, branch, base string, pick Pick) (Plan, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return Plan{}, errors.New("name the branch")
	}
	if BadBranch(branch) {
		return Plan{}, fmt.Errorf("%q isn't a valid branch name", branch)
	}
	plan := Plan{Branch: branch}
	if src.GitHub != "" {
		switch pick.Kind {
		case "issue":
			plan.IssueRef = src.GitHub + "#" + strconv.Itoa(pick.Number)
			if pick.PR > 0 && branch == pick.Head {
				plan.PRRef = src.GitHub + "#" + strconv.Itoa(pick.PR)
			}
		case "pr":
			if branch == pick.Head {
				plan.PRRef = src.GitHub + "#" + strconv.Itoa(pick.Number)
			}
		}
	}
	for _, c := range src.Checkouts {
		if c.Branch == branch {
			plan.Action, plan.Path = "open", c.Path
			return plan, nil
		}
	}
	if hasBranch(src.Local, branch) {
		plan.Action = "local"
		return plan, nil
	}
	if hasBranch(src.Remote, branch) {
		plan.Action, plan.From, plan.Track = "remote", "origin/"+branch, true
		return plan, nil
	}
	prNumber, head := pick.Number, pick.Head
	if pick.Kind == "issue" {
		prNumber = pick.PR
	}
	headPicked := (pick.Kind == "pr" || pick.Kind == "issue") && head != "" && prNumber > 0
	if headPicked && branch == head {
		if pick.CrossRepo {
			plan.Action, plan.Fetch = "local", "+refs/pull/"+strconv.Itoa(prNumber)+"/head:refs/heads/"+branch
			return plan, nil
		}
		plan.Action, plan.From, plan.Track, plan.Fetch = "remote", "origin/"+branch, true, "+refs/heads/"+branch+":refs/remotes/origin/"+branch
		return plan, nil
	}
	plan.Action = "new"
	switch {
	case pick.Kind == "branch" && hasBranch(src.Local, pick.Branch):
		plan.From = pick.Branch
	case pick.Kind == "branch" && hasBranch(src.Remote, pick.Branch):
		plan.From = "origin/" + pick.Branch
	case pick.Kind == "pr" && head != "" && !pick.CrossRepo:
		plan.From = "origin/" + head
		if !hasBranch(src.Remote, head) {
			plan.Fetch = "+refs/heads/" + head + ":refs/remotes/origin/" + head
		}
	default:
		plan.From = strings.TrimSpace(base)
		if plan.From == "" {
			plan.From = src.Base
		}
	}
	if BadBranch(plan.From) && plan.From != "HEAD" {
		return Plan{}, fmt.Errorf("%q isn't a branch or commit to start from", plan.From)
	}
	plan.Untrack = strings.HasPrefix(plan.From, "origin/")
	return plan, nil
}

func hasBranch(list []repos.Branch, name string) bool {
	for _, b := range list {
		if b.Name == name {
			return true
		}
	}
	return false
}

// Slug is a branch as herdr names its worktree's folder: lower case, a dash for every other run of characters.
func Slug(branch string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(branch) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	return b.String()
}

// FolderFor is where a new worktree of the branch goes: in the project's own location, else "" for herdr's root.
func FolderFor(location, branch string) string {
	if location == "" {
		return ""
	}
	return rpath.For(location).Join(location, Slug(branch))
}

// Maker is what a create needs of herdr.
type Maker interface {
	WorktreeCreateAt(workspaceID, branch, base, label, path string) (*herdr.WorktreeCreateResult, error)
	WorktreeOpen(workspaceID, path, label string) (*herdr.WorktreeCreateResult, error)
}

// Created is a create's outcome: herdr's answer, and what of the git around it did not go through.
type Created struct {
	*herdr.WorktreeCreateResult
	Notes []string
}

// Create carries the plan out in the repository through herdr: the fetch first, the checkout, then its upstream and links.
func Create(host shell.Shell, h Maker, root, workspaceID string, plan Plan, label, folder string) (Created, error) {
	if plan.Fetch != "" {
		if out, err := shell.Text(host, fetchScript(root, plan.Fetch), FetchTimeout); err != nil {
			return Created{}, fmt.Errorf("git fetch: %s", lastLine(out, err.Error()))
		}
	}
	var (
		res *herdr.WorktreeCreateResult
		err error
	)
	switch plan.Action {
	case "open":
		res, err = h.WorktreeOpen(workspaceID, plan.Path, label)
	case "local":
		res, err = h.WorktreeCreateAt(workspaceID, plan.Branch, "", label, folder)
	default:
		res, err = h.WorktreeCreateAt(workspaceID, plan.Branch, plan.From, label, folder)
	}
	if err != nil {
		return Created{}, err
	}
	made := Created{WorktreeCreateResult: res}
	if after := afterScript(root, plan); after != "" {
		if out, err := shell.Text(host, after, Timeout); err != nil {
			made.Notes = append(made.Notes, "git config: "+lastLine(out, err.Error()))
		}
	}
	return made, nil
}

// fetchScript fetches one refspec from origin into the repository.
func fetchScript(root, refspec string) string {
	return script.PATH + "\ncd " + script.Quote(root) + " 2>/dev/null || { echo 'the repository is not there'; exit 3; }\n" +
		"git fetch origin " + script.Quote(refspec) + " 2>&1\n"
}

// afterScript sets the new branch's upstream as the plan says and notes the issue and pull request it was made for, where
// the sidebar's badges and the agents can read them; "" when there is nothing to set.
func afterScript(root string, plan Plan) string {
	b := plan.Branch
	var lines []string
	if plan.Track {
		lines = append(lines, "git branch --set-upstream-to="+script.Quote("origin/"+b)+" "+script.Quote(b)+" >/dev/null 2>&1 || true")
	}
	if plan.Untrack {
		lines = append(lines, "git branch --unset-upstream "+script.Quote(b)+" >/dev/null 2>&1 || true")
	}
	if plan.IssueRef != "" {
		lines = append(lines, "git config "+script.Quote("branch."+b+".remuda-issue")+" "+script.Quote(plan.IssueRef)+" 2>&1 || exit 6")
	}
	if plan.PRRef != "" {
		lines = append(lines, "git config "+script.Quote("branch."+b+".remuda-pr")+" "+script.Quote(plan.PRRef)+" 2>&1 || exit 6")
	}
	if len(lines) == 0 {
		return ""
	}
	return script.PATH + "\ncd " + script.Quote(root) + " 2>/dev/null || { echo 'the repository is not there'; exit 3; }\n" + strings.Join(lines, "\n") + "\n"
}
