// Package viewer is what a viewer link may read of a machine's files: the workspaces' folders, never the places a key
// lives in.
package viewer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dev1antdev/remuda/internal/rpath"
)

// secretFiles are the files of a state dir that hold a credential.
var secretFiles = map[string]bool{
	"token": true, "webhooks.json": true, "vapid.json": true, "jira.json": true,
	"push-subscriptions.json": true, "devices.json": true,
}

// closedName says the file is one of remuda's own secrets by its name, wherever a state dir was put: those above, and an
// agent key with the files it is written through.
func closedName(name string) bool {
	return secretFiles[name] || strings.HasPrefix(name, "agent-key")
}

// Reach is what a viewer may read on one machine.
type Reach struct {
	folders, closed []string
	home            string
	style           rpath.Style
	local           bool
}

// Of is the reach of the workspace folders given: those that hold no home, since one at a home or above it would open
// everything there, less the machine's ~/.ssh and remuda's state dir, the one it reports and the usual ones, where the
// agents' door may stand whatever the service uses. A machine not read yet reaches nothing.
func Of(dirs []string, home, stateDir string, style rpath.Style, local bool) Reach {
	reach := Reach{style: style, local: local}
	if home == "" || stateDir == "" {
		return reach
	}
	for _, dir := range dirs {
		if !style.HoldsAHome(folded(home, local), folded(dir, local)) {
			reach.folders = append(reach.folders, dir)
		}
	}
	reach.home = home
	reach.closed = []string{
		style.Join(home, ".ssh"),
		stateDir,
		style.Join(home, ".config", "remuda"),
		style.Join(home, "Library", "Application Support", "remuda"),
		style.Join(home, "AppData", "Roaming", "remuda"),
	}
	return reach
}

// Allows says the viewer may read the path.
func (r Reach) Allows(path string) bool {
	for _, folder := range r.folders {
		if r.style.Within(realPath(folder, r.local), realPath(path, r.local)) {
			return !r.inClosed(path)
		}
	}
	return false
}

// Closes says the path is a place no viewer reads, whichever folder it is asked through; on a machine not read yet, any.
func (r Reach) Closes(path string) bool {
	return r.home == "" || r.inClosed(path)
}

// AllowsCheckout says the viewer may see the history of the checkout at top, "" for a folder outside git: a commit names
// files of every folder of it, so it may hold neither a home nor a closed place.
func (r Reach) AllowsCheckout(top string) bool {
	if top == "" {
		return true
	}
	if r.home == "" || r.style.HoldsAHome(folded(r.home, r.local), folded(top, r.local)) {
		return false
	}
	for _, closed := range r.closed {
		if r.style.Within(folded(top, r.local), folded(closed, r.local)) {
			return false
		}
	}
	return true
}

// OpenLocal opens a file of this machine the viewer may read. The path is resolved once, and that target is what is
// judged, by Allows and by refused, what is opened, inside its workspace folder, and what the open file must still be:
// a link swapped in at any moment gives another file, or another target, and is refused.
func (r Reach) OpenLocal(path string, refused func(string) bool) (*os.File, error) {
	real := realPath(path, true)
	if !r.Allows(real) || refused(real) {
		return nil, fs.ErrPermission
	}
	for _, folder := range r.folders {
		root := realPath(folder, true)
		rel, ok := r.style.Rel(root, real)
		if !ok {
			continue
		}
		f, err := os.OpenInRoot(root, filepath.FromSlash(rel))
		if err != nil {
			return nil, err
		}
		opened, err := f.Stat()
		if err == nil && opened.Mode().IsRegular() && realPath(path, true) == real {
			if now, err := os.Lstat(real); err == nil && os.SameFile(opened, now) {
				return f, nil
			}
		}
		f.Close()
		return nil, fs.ErrPermission
	}
	return nil, fs.ErrPermission
}

func (r Reach) inClosed(path string) bool {
	if closedName(strings.ToLower(r.style.Base(realPath(path, r.local)))) {
		return true
	}
	for _, closed := range r.closed {
		if r.style.Within(folded(closed, r.local), folded(path, r.local)) {
			return true
		}
	}
	return false
}

// PatchAllowed says a viewer may read a commit's patch: none of the files it shows, on either side of a rename, under
// any folder asked for, is one of remuda's secrets or lies in a .ssh or a remuda folder, as any commit may have kept one.
// The names are read from each file's header, where a line holds one path whole; the changed lines are left alone. A
// patch with no header this can read shows nothing it can vouch for.
func PatchAllowed(patch string) bool {
	inHeader, read := false, false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHeader, read = true, true
		case strings.HasPrefix(line, "@@ "):
			inHeader = false
		case inHeader:
			for _, label := range []string{"--- ", "+++ ", "rename from ", "rename to ", "copy from ", "copy to "} {
				if name, ok := strings.CutPrefix(line, label); ok && closedPatchPath(name) {
					return false
				}
			}
		}
	}
	return read || strings.TrimSpace(patch) == ""
}

// closedPatchPath reads a path as a patch header spells it, quoted when git had to, with a tab after a name with spaces.
func closedPatchPath(path string) bool {
	path = strings.ToLower(strings.Trim(strings.TrimRight(path, "\t\r"), `"`))
	for _, part := range strings.Split(path, "/") {
		if closedName(part) || part == ".ssh" || part == "remuda" || part == "serve.lock" {
			return true
		}
	}
	return false
}

// folded is the path as compared where a Mac's disk would find it in any case.
func folded(path string, local bool) string { return strings.ToLower(realPath(path, local)) }

// realPath is the path with its symlinks resolved on this machine, so a link out of a workspace reaches nothing and
// /tmp and /private/tmp are one folder; the name is kept when the file is not there yet.
func realPath(path string, local bool) string {
	if !local {
		return path
	}
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(dir, filepath.Base(path))
	}
	return path
}
