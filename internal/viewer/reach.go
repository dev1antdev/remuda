// Package viewer is what a viewer link may read of a machine's files: the workspaces' folders, never the places a key
// lives in.
package viewer

import (
	"path/filepath"
	"strings"

	"github.com/dev1antdev/remuda/internal/rpath"
)

// Reach is what a viewer may read on one machine.
type Reach struct {
	folders, closed []string
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

// AllowsAll says the viewer may read the folder and whatever lies under it: what a checkout's history needs, which
// names files of every folder of it.
func (r Reach) AllowsAll(folder string) bool {
	if !r.Allows(folder) {
		return false
	}
	for _, closed := range r.closed {
		if r.style.Within(folded(folder, r.local), folded(closed, r.local)) {
			return false
		}
	}
	return true
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
