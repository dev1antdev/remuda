// Package gitops runs git's writes on a checkout at the person's press: a checkout, a branch made, renamed or deleted, a
// merge, a rebase, a cherry-pick, a revert, a reset, a tag added or deleted, and the abort of a merge, rebase,
// cherry-pick or revert that stopped on conflicts. Each op is validated before any script runs, refused while an agent
// works, asks or ends a turn in that checkout, and refused when HEAD or the branch moved since the page drew them; git's
// own last lines come back when it fails. Nothing here fetches, pulls or pushes, and nothing ever forces.
package gitops

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/gitgraph"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/shell"
)

const runFor = 2 * time.Minute

// Request is one op as the page asks for it, with HEAD and the branch as the page saw them.
type Request struct {
	Op        string `json:"op"`
	Ref       string `json:"ref,omitempty"`  // an existing branch, remote branch or tag
	Name      string `json:"name,omitempty"` // a new branch's or tag's name
	Sha       string `json:"sha,omitempty"`  // a commit by its hash
	Mode      string `json:"mode,omitempty"` // a reset's: soft | mixed | hard
	Message   string `json:"message,omitempty"`
	Checkout  bool   `json:"checkout,omitempty"` // a new branch checked out at once
	NoFF      bool   `json:"no_ff,omitempty"`    // a merge commit even when it could fast-forward
	Force     bool   `json:"force,omitempty"`    // a branch deleted though it is not merged
	Head      string `json:"head"`
	Branch    string `json:"branch"`
	Operation string `json:"operation,omitempty"` // for an abort: the op the page saw stopped
}

// Result is how the op went: git's last lines when it failed, an operation left in progress by conflicts, a branch
// that -d would not delete because it is not merged.
type Result struct {
	OK        bool   `json:"ok"`
	Output    string `json:"output,omitempty"`
	Operation string `json:"operation,omitempty"`
	Conflicts int    `json:"conflicts,omitempty"`
	NotMerged bool   `json:"not_merged,omitempty"`
}

// Checkout is where a folder's checkout stands before an op: its top, HEAD, branch, a stopped op, and the tops of every
// worktree of its repository, so an agent in another worktree nested inside it is not taken for one in it.
type Checkout struct {
	Top       string
	Head      string
	Branch    string
	Operation string
	Worktrees []string
}

// InvalidError is a request refused before any script ran.
type InvalidError struct{ Reason string }

func (e *InvalidError) Error() string { return e.Reason }

// BusyError is an op refused because an agent works, asks or is ending a turn in the checkout.
type BusyError struct{ Who string }

func (e *BusyError) Error() string {
	return e.Who + " in this checkout: try again once it has finished"
}

// ErrStale is an op on a checkout whose HEAD or branch moved since the page drew them.
var ErrStale = errors.New("the checkout's branch or HEAD moved since the graph was drawn: look at it again first")

// ErrNotRepository is a folder outside git, or gone.
var ErrNotRepository = errors.New("this folder is not inside a git repository")

// Service runs the ops; Busy names who is at work in the checkout ("" for nobody), wired from the watcher's snapshot
// and the undo tracker.
type Service struct {
	Busy func(machineID, top string, worktrees []string) string
}

var resetModes = map[string]bool{"soft": true, "mixed": true, "hard": true}

var aborts = map[string]bool{"merge": true, "rebase": true, "cherry-pick": true, "revert": true}

// nameOK is a ref name git takes as one and the shell never reads as an option: sessions.RefOK, and none of the
// characters or spellings git itself refuses in a ref.
func nameOK(name string) bool {
	if !sessions.RefOK(name) || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") || strings.Contains(name, "@{") || name == "@" {
		return false
	}
	if strings.ContainsAny(name, "~^:?*[\\") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

// Validate refuses a request whose op is unknown or whose arguments git would not read as the page means them.
func Validate(req Request) error {
	bad := func(reason string) error { return &InvalidError{Reason: reason} }
	needRef := func() error {
		if !nameOK(req.Ref) {
			return bad("that branch or tag name cannot be passed to git")
		}
		return nil
	}
	needName := func() error {
		if !nameOK(strings.TrimSpace(req.Name)) {
			return bad("a name may not start with -, hold a space, .., ~ ^ : ? * [ \\ or end in / . or .lock")
		}
		return nil
	}
	needSha := func() error {
		if !gitgraph.HashOK(req.Sha) {
			return bad("a commit is named by its hash")
		}
		return nil
	}
	if req.Head != "" && !gitgraph.HashOK(req.Head) {
		return bad("HEAD is named by its hash")
	}
	switch req.Op {
	case "checkout", "branch-delete", "tag-delete":
		return needRef()
	case "checkout-remote", "branch-rename":
		if err := needRef(); err != nil {
			return err
		}
		return needName()
	case "checkout-commit", "cherry-pick", "revert":
		return needSha()
	case "branch-create", "tag-add":
		if err := needSha(); err != nil {
			return err
		}
		if err := needName(); err != nil {
			return err
		}
		if strings.ContainsAny(req.Message, "\x00") {
			return bad("the message holds a NUL")
		}
		return nil
	case "merge", "rebase":
		if gitgraph.HashOK(req.Sha) {
			return nil
		}
		return needRef()
	case "reset":
		if !resetModes[req.Mode] {
			return bad("a reset is soft, mixed or hard")
		}
		return needSha()
	case "abort":
		if !aborts[req.Operation] {
			return bad("only a merge, rebase, cherry-pick or revert in progress can be aborted")
		}
		return nil
	}
	return bad(fmt.Sprintf("git has no op %q here", req.Op))
}

// Command is the git invocation an op makes, every word quoted for the shell; a cherry-pick and a revert of a merge
// commit take its first parent as the mainline, which the script decides ($mainline).
func Command(req Request) string {
	q := script.Quote
	name := strings.TrimSpace(req.Name)
	target := req.Ref // a merge's or a rebase's: a branch, or a commit by its hash
	if gitgraph.HashOK(req.Sha) {
		target = req.Sha
	}
	switch req.Op {
	case "checkout":
		return "w checkout " + q(req.Ref) + " --"
	case "checkout-remote":
		return "w checkout -b " + q(name) + " --track " + q("refs/remotes/"+req.Ref)
	case "checkout-commit":
		return "w checkout --detach " + q(req.Sha)
	case "branch-create":
		if req.Checkout {
			return "w checkout -b " + q(name) + " " + q(req.Sha)
		}
		return "w branch " + q(name) + " " + q(req.Sha)
	case "branch-rename":
		return "w branch -m " + q(req.Ref) + " " + q(name)
	case "branch-delete":
		if req.Force {
			return "w branch -D " + q(req.Ref)
		}
		return "w branch -d " + q(req.Ref)
	case "merge":
		noff := ""
		if req.NoFF {
			noff = "--no-ff "
		}
		return "w merge --no-edit " + noff + q(target)
	case "rebase":
		return "w rebase " + q(target)
	case "cherry-pick":
		return "w cherry-pick $mainline " + q(req.Sha)
	case "revert":
		return "w revert --no-edit $mainline " + q(req.Sha)
	case "reset":
		return "w reset --" + req.Mode + " " + q(req.Sha)
	case "tag-add":
		if message := strings.TrimSpace(req.Message); message != "" {
			return "w tag -a " + q(name) + " -m " + q(message) + " " + q(req.Sha)
		}
		return "w tag " + q(name) + " " + q(req.Sha)
	case "tag-delete":
		return "w tag -d " + q(req.Ref)
	case "abort":
		return "w " + req.Operation + " --abort"
	}
	return "false"
}

// Inspect reads where the folder's checkout stands before an op.
func Inspect(host shell.Host, dir string) (Checkout, error) {
	out, err := shell.Text(host, script.PATH+"; "+gitgraph.Read+
		`cd `+script.Quote(dir)+` 2>/dev/null || exit 0; top=$(g rev-parse --show-toplevel 2>/dev/null) || exit 0; gd=$(g rev-parse --absolute-git-dir 2>/dev/null); `+
		`op=; if [ -d "$gd/rebase-merge" ] || [ -d "$gd/rebase-apply" ]; then op=rebase; elif [ -f "$gd/MERGE_HEAD" ]; then op=merge; `+
		`elif [ -f "$gd/CHERRY_PICK_HEAD" ]; then op=cherry-pick; elif [ -f "$gd/REVERT_HEAD" ]; then op=revert; fi; `+
		`printf 'checkout\n%s\n%s\n%s\n%s\n' "$top" "$(g rev-parse -q --verify HEAD 2>/dev/null)" "$(g symbolic-ref -q --short HEAD 2>/dev/null)" "$op"; `+
		`g worktree list --porcelain 2>/dev/null | sed -n 's/^worktree //p'`, 30*time.Second)
	if err != nil {
		return Checkout{}, err
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 5 || lines[0] != "checkout" || lines[1] == "" {
		return Checkout{}, ErrNotRepository
	}
	at := Checkout{Top: lines[1], Head: lines[2], Branch: lines[3], Operation: lines[4]}
	for _, line := range lines[5:] {
		if line != "" {
			at.Worktrees = append(at.Worktrees, line)
		}
	}
	return at, nil
}

// Run does one op on the folder's checkout, the person having pressed it and, where it loses or rewrites anything,
// confirmed it on the page.
func (s *Service) Run(host shell.Host, dir string, req Request) (Result, error) {
	if err := Validate(req); err != nil {
		return Result{}, err
	}
	at, err := Inspect(host, dir)
	if err != nil {
		return Result{}, err
	}
	if s.Busy != nil {
		if who := s.Busy(host.ID(), at.Top, at.Worktrees); who != "" {
			return Result{}, &BusyError{Who: who}
		}
	}
	if req.Op == "abort" {
		if at.Operation != req.Operation {
			return Result{}, ErrStale
		}
	} else if at.Head != req.Head || at.Branch != req.Branch {
		return Result{}, ErrStale
	}
	out, err := shell.Text(host, runScript(dir, req), runFor)
	if err != nil {
		return Result{}, err
	}
	return parseRun(out)
}

// runScript checks HEAD and the branch once more right before the write, then runs it with no editor, no prompt and
// git's words in English, and prints how it ended and where the checkout stands after it.
func runScript(dir string, req Request) string {
	q := script.Quote
	guard := `[ "$(g rev-parse -q --verify HEAD 2>/dev/null)" = ` + q(req.Head) + ` ] && [ "$(g symbolic-ref -q --short HEAD 2>/dev/null)" = ` + q(req.Branch) + ` ] || { printf '\036stale\n'; exit 0; }; `
	if req.Op == "abort" {
		guard = ""
	}
	mainline := ""
	if req.Op == "cherry-pick" || req.Op == "revert" {
		mainline = `mainline=; [ "$(g rev-list --parents -n 1 ` + q(req.Sha) + ` 2>/dev/null | wc -w)" -gt 2 ] && mainline="-m 1"; `
	}
	return script.PATH + "; " + gitgraph.Read +
		`w() { GIT_TERMINAL_PROMPT=0 GIT_EDITOR=true GIT_SEQUENCE_EDITOR=true GIT_MERGE_AUTOEDIT=no LC_ALL=C git -c core.editor=true -c color.ui=never --no-pager "$@"; }; ` +
		`cd ` + q(dir) + ` 2>/dev/null || { printf '\036gone\n'; exit 0; }; ` + guard + mainline +
		`out=$(` + Command(req) + ` 2>&1); code=$?; printf '\036code %s\n' "$code"; printf '%s\n' "$out" | tail -n 40; ` +
		`gd=$(g rev-parse --absolute-git-dir 2>/dev/null); op=; if [ -d "$gd/rebase-merge" ] || [ -d "$gd/rebase-apply" ]; then op=rebase; elif [ -f "$gd/MERGE_HEAD" ]; then op=merge; ` +
		`elif [ -f "$gd/CHERRY_PICK_HEAD" ]; then op=cherry-pick; elif [ -f "$gd/REVERT_HEAD" ]; then op=revert; fi; ` +
		`printf '\036after %s %s\n' "${op:--}" "$(g diff --name-only --diff-filter=U 2>/dev/null | wc -l | tr -d ' ')"; printf '\036end\n'`
}

// parseRun reads what runScript printed.
func parseRun(out string) (Result, error) {
	chunks := strings.Split(out, script.RS)
	var result Result
	code := -1
	for _, chunk := range chunks[1:] {
		line, body, _ := strings.Cut(chunk, "\n")
		name, args, _ := strings.Cut(line, " ")
		switch name {
		case "gone":
			return Result{}, ErrNotRepository
		case "stale":
			return Result{}, ErrStale
		case "code":
			code, _ = strconv.Atoi(args)
			result.Output = gitgraph.LastLines(body, 8)
		case "after":
			op, conflicts, _ := strings.Cut(args, " ")
			if op != "-" {
				result.Operation = op
			}
			result.Conflicts, _ = strconv.Atoi(strings.TrimSpace(conflicts))
		}
	}
	if code < 0 {
		return Result{}, errors.New("git did not run: " + gitgraph.LastLines(out, 4))
	}
	result.OK = code == 0
	result.NotMerged = !result.OK && strings.Contains(result.Output, "not fully merged")
	return result, nil
}
