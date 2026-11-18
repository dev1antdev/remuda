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

// closedNames are remuda's own secrets by name, for a state dir kept where the machine's probe cannot see it.
var closedNames = map[string]bool{"token": true, "agent-key": true, "agent-keys.json": true}

// Reach is what a viewer may read on one machine.
type Reach struct {
	folders, closed []string
	home            string
	style           rpath.Style
	local           bool
}

// Of is the reach of the workspace folders given: those that hold no home, since one at a home or above it would open
// everything there, less the machine's ~/.ssh and remuda's state dir. A machine not read yet reaches nothing.
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
	reach.closed = []string{style.Join(home, ".ssh"), stateDir}
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

// OpenLocal opens a file of this machine the viewer may read, inside its workspace folder by the system's own check: a
// link swapped in after Allows looked leads nowhere out of that folder.
func (r Reach) OpenLocal(path string) (*os.File, error) {
	real := realPath(path, true)
	for _, folder := range r.folders {
		root := realPath(folder, true)
		if rel, ok := r.style.Rel(root, real); ok && r.Allows(path) {
			return os.OpenInRoot(root, filepath.FromSlash(rel))
		}
	}
	return nil, fs.ErrPermission
}

// Allowed keeps the paths the viewer may read.
func (r Reach) Allowed(paths []string) []string {
	var kept []string
	for _, path := range paths {
		if r.Allows(path) {
			kept = append(kept, path)
		}
	}
	return kept
}

func (r Reach) inClosed(path string) bool {
	if closedNames[strings.ToLower(r.style.Base(realPath(path, r.local)))] {
		return true
	}
	for _, closed := range r.closed {
		if r.style.Within(folded(closed, r.local), folded(path, r.local)) {
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
