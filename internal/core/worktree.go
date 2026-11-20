package core

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/text"
)

// The core knows nothing of git: on a remuda-core machine the adapter makes and reads worktrees itself, with git run
// through the machine's shell, in herdr's place (~/.herdr/worktrees/<repository>/<branch's slug>, the root
// worktrees.Root names) so the ones made before and after the move lie together.

// How long a worktree may take to be made (a big checkout), and a read of one.
const (
	worktreeWithin = 10 * time.Minute
	checkoutWithin = 20 * time.Second
)

// probeFunc prints the checkout a folder is — its top, git's common and own folders, the repository's main folder as
// the sidebar's sweep reads it (script.MainFolder) — or nothing outside git (or on a git before 2.31, which has no
// --path-format).
const probeFunc = `probe() ( cd "$1" 2>/dev/null || exit 0
  top=$(` + script.ReadGit + ` rev-parse --show-toplevel 2>/dev/null) || exit 0
  common=$(` + script.ReadGit + ` rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || exit 0
  own=$(` + script.ReadGit + ` rev-parse --path-format=absolute --git-dir 2>/dev/null) || exit 0
  printf 'top %s\ncommon %s\nown %s\nmain %s\n' "$top" "$common" "$own" "$(` + script.MainFolder + `)"
)
`

// checkoutScript reads the checkout a folder is.
func checkoutScript(folder string) string {
	return script.PATH + "\n" + probeFunc + "probe " + script.Quote(folder) + "\n"
}

// addScript makes a worktree of the checkout at dir for the branch, in path or herdr's place for it, named after the
// repository's main folder: with a base, the branch is new from it and git refuses one that is there already (so no
// branch of the person's is taken over); without, a branch that exists is checked out, else it starts at dir's HEAD,
// as herdr's worktree.create does. A checkout of this repository already on the branch at that very place — an earlier
// attempt whose workspace did not open — is taken as it is. It prints "path <folder>" and the new checkout as probe
// reads it; git's own words when it refuses.
func addScript(dir, branch, base, path string) string {
	return script.PATH + "\n" + probeFunc +
		"cd " + script.Quote(dir) + " 2>/dev/null || { echo 'the workspace folder is not there'; exit 3; }\n" +
		script.ReadGit + " rev-parse --show-toplevel >/dev/null 2>&1 || { echo 'the workspace folder is not a git checkout'; exit 3; }\n" +
		"b=" + script.Quote(branch) + "; base=" + script.Quote(base) + "; p=" + script.Quote(path) + "\n" +
		`[ -n "$p" ] || p="$HOME/.herdr/worktrees/$(basename "$(` + script.MainFolder + `)")/"` + script.Quote(text.Slug(branch)) + "\n" +
		`common() { ` + script.ReadGit + ` -C "$1" rev-parse --path-format=absolute --git-common-dir 2>/dev/null; }` + "\n" +
		`if [ "$(` + script.ReadGit + ` -C "$p" symbolic-ref -q --short HEAD 2>/dev/null)" = "$b" ] && [ "$(common "$p")" = "$(common .)" ]; then :` + "\n" +
		`else mkdir -p "$(dirname "$p")" || exit 4` + "\n" +
		`  if [ -n "$base" ]; then git worktree add -q -b "$b" "$p" "$base" 2>&1 || exit 5` + "\n" +
		`  elif git show-ref -q --verify "refs/heads/$b"; then git worktree add -q "$p" "$b" 2>&1 || exit 5` + "\n" +
		`  else git worktree add -q -b "$b" "$p" HEAD 2>&1 || exit 5; fi` + "\n" +
		"fi\n" +
		`echo "path $p"` + "\n" + `probe "$p"` + "\n"
}

// parseCheckout reads probe's lines: nil when the folder is in no checkout.
func parseCheckout(out string) (path string, tree *herdr.WorktreeInfo) {
	fields := map[string]string{}
	for line := range strings.Lines(out) {
		if key, value, ok := strings.Cut(strings.TrimRight(line, "\r\n"), " "); ok {
			fields[key] = value
		}
	}
	top := fields["top"]
	if top == "" || fields["common"] == "" {
		return fields["path"], nil
	}
	style := rpath.For(top)
	main := style.Clean(cmp.Or(fields["main"], top))
	return fields["path"], &herdr.WorktreeInfo{
		CheckoutPath:     style.Clean(top),
		IsLinkedWorktree: !style.Equal(fields["own"], fields["common"]),
		RepoKey:          style.Clean(fields["common"]),
		RepoName:         style.Base(main),
		RepoRoot:         main,
	}
}

// checkoutOf reads the checkout a folder is, once, when a workspace is made on it; nil outside git, without a shell,
// or when the read failed (the workspace is made all the same).
func (a *Adapter) checkoutOf(folder string) *herdr.WorktreeInfo {
	if a.shell == nil || folder == "" {
		return nil
	}
	out, err := shell.Text(a.shell, checkoutScript(rpath.For(folder).Slash(folder)), checkoutWithin)
	if err != nil {
		return nil
	}
	_, tree := parseCheckout(out)
	return tree
}

// WorktreeCreate makes a worktree of the workspace's repository in herdr's place and opens it as a workspace.
func (a *Adapter) WorktreeCreate(workspaceID, branch, base, label string) (*herdr.WorktreeCreateResult, error) {
	return a.WorktreeCreateAt(workspaceID, branch, base, label, "")
}

// WorktreeCreateAt is WorktreeCreate in path, "" for herdr's place; a base makes the branch new (addScript).
func (a *Adapter) WorktreeCreateAt(workspaceID, branch, base, label, path string) (*herdr.WorktreeCreateResult, error) {
	dir, err := a.sourceDir(workspaceID)
	if err != nil {
		return nil, err
	}
	style := rpath.For(dir)
	if path != "" {
		path = style.Slash(path)
	}
	out, err := shell.Text(a.shell, addScript(style.Slash(dir), branch, base, path), worktreeWithin)
	if err != nil {
		return nil, fmt.Errorf("git worktree add: %s", text.LastLine(out, err.Error()))
	}
	made, tree := parseCheckout(out)
	folder := made
	if tree != nil {
		folder = tree.CheckoutPath
	}
	res, err := a.workspaceOn(folder, cmp.Or(label, rpath.For(folder).Base(folder)), tree)
	if err != nil {
		return nil, fmt.Errorf("the worktree %s is made, but its workspace did not open (making it again opens it): %w", folder, err)
	}
	created := &herdr.WorktreeCreateResult{Workspace: res.Workspace, RootPane: res.RootPane}
	created.Worktree.Path, created.Worktree.Branch = folder, branch
	return created, nil
}

// WorktreeOpen opens a checkout of the workspace's repository as a workspace; one open on it already is answered, as
// herdr's already_open, from the mirror — the route's own look reads the watcher's snapshot, which may lag a moment.
func (a *Adapter) WorktreeOpen(workspaceID, path, label string) (*herdr.WorktreeCreateResult, error) {
	if _, err := a.sourceDir(workspaceID); err != nil {
		return nil, err
	}
	tree := a.checkoutOf(path)
	if tree == nil {
		return nil, &herdr.Error{Code: "invalid_request", Message: path + " is not a git checkout"}
	}
	a.opening.Lock()
	defer a.opening.Unlock()
	opened := &herdr.WorktreeCreateResult{}
	opened.Worktree.Path = tree.CheckoutPath
	snap, err := a.SessionSnapshot()
	if err != nil {
		return nil, err
	}
	style := rpath.For(tree.CheckoutPath)
	for i := range snap.Workspaces {
		if ws := &snap.Workspaces[i]; style.Equal(snap.WorkspaceDir(ws), tree.CheckoutPath) {
			opened.Workspace, opened.RootPane, opened.AlreadyOpen = *ws, frontPane(snap, ws), true
			return opened, nil
		}
	}
	res, err := a.workspaceOn(tree.CheckoutPath, cmp.Or(label, style.Base(tree.CheckoutPath)), tree)
	if err != nil {
		return nil, err
	}
	opened.Workspace, opened.RootPane = res.Workspace, res.RootPane
	return opened, nil
}

// frontPane is a workspace's pane in its tab in front, else its first.
func frontPane(snap *herdr.Snapshot, ws *herdr.WorkspaceInfo) herdr.PaneInfo {
	pane := herdr.PaneInfo{WorkspaceID: ws.WorkspaceID}
	for _, p := range snap.Panes {
		if p.WorkspaceID != ws.WorkspaceID {
			continue
		}
		if p.TabID == ws.ActiveTabID {
			return p
		}
		if pane.PaneID == "" {
			pane = p
		}
	}
	return pane
}

// sourceDir is the folder of the workspace a worktree is made from.
func (a *Adapter) sourceDir(workspaceID string) (string, error) {
	if a.shell == nil {
		return "", &herdr.Error{Code: "invalid_request", Message: "no shell to run git on this remuda-core machine"}
	}
	snap, err := a.SessionSnapshot()
	if err != nil {
		return "", err
	}
	ws := snap.Workspace(workspaceID)
	if ws == nil {
		return "", spaceMissing(workspaceID)
	}
	return snap.WorkspaceDir(ws), nil
}
