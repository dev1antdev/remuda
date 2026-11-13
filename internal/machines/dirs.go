package machines

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
)

// DirEntry is one subfolder of a listed folder.
type DirEntry struct {
	Name string `json:"name"`
	Git  bool   `json:"git"` // a repository (a .git inside)
}

// DirListing is a folder and its subfolders, for the folder picker.
type DirListing struct {
	Path   string     `json:"path"`             // resolved, absolute, in the machine's spelling
	Parent string     `json:"parent,omitempty"` // "" at the root
	Home   string     `json:"home,omitempty"`
	Dirs   []DirEntry `json:"dirs"`
}

// listDirsScript lists a folder's subfolders: the folder and home first, then a name and its .git mark a line. On a
// Windows machine both are printed in Windows' spelling, which herdr takes as a new workspace's folder.
func listDirsScript(path string, hidden, windows bool) string {
	glob := "*/"
	if hidden {
		glob = "*/ .*/"
	}
	here, home := `"$(pwd -P)"`, `"$HOME"`
	if windows {
		here, home = `"$(cygpath -w "$(pwd -P)")"`, `"$(cygpath -w "$HOME")"`
	}
	return `p=` + script.Quote(path) + `; [ -z "$p" ] && p="$HOME"; case "$p" in "~") p="$HOME";; "~/"*) p="$HOME/${p#"~/"}";; esac; ` +
		`cd "$p" 2>/dev/null || { printf 'ERR\037not a folder: %s\n' "$p"; exit 0; }; printf '%s\n' ` + here + `; printf '%s\n' ` + home + `; printf '\036\n'; ` +
		`for d in ` + glob + `; do case "$d" in ./|../) continue;; esac; [ -d "$d" ] || continue; n=${d%/}; g=0; [ -e "$d.git" ] && g=1; printf '%s\037%s\n' "$n" "$g"; done`
}

// ListDirs lists the subfolders of path on the machine ("" or "~" is home), hidden ones only when asked.
func (mc *Machine) ListDirs(path string, hidden bool) (DirListing, error) {
	style := mc.Paths()
	out, err := mc.text(listDirsScript(path, hidden, style.Windows), 30*time.Second)
	if err != nil {
		return DirListing{}, err
	}
	return readListing(out, style.Windows)
}

// readListing reads listDirsScript's answer.
func readListing(out string, windows bool) (DirListing, error) {
	if strings.HasPrefix(out, "ERR\x1f") {
		return DirListing{}, fmt.Errorf("%s", strings.TrimSpace(strings.TrimPrefix(out, "ERR\x1f")))
	}
	style := rpath.Style{Windows: windows}
	head, rest, _ := strings.Cut(out, "\x1e\n")
	lines := strings.Split(strings.TrimRight(head, "\r\n"), "\n")
	l := DirListing{Dirs: []DirEntry{}}
	if len(lines) > 0 {
		l.Path = style.Clean(strings.TrimRight(lines[0], "\r"))
	}
	if len(lines) > 1 {
		l.Home = style.Clean(strings.TrimRight(lines[1], "\r"))
	}
	if parent := style.Dir(l.Path); l.Path != "" && parent != l.Path {
		l.Parent = parent
	}
	for _, line := range strings.Split(strings.TrimRight(rest, "\n"), "\n") {
		if n, g, ok := strings.Cut(strings.TrimRight(line, "\r"), "\x1f"); ok && n != "" {
			l.Dirs = append(l.Dirs, DirEntry{Name: n, Git: g == "1"})
		}
	}
	sort.Slice(l.Dirs, func(i, j int) bool { return strings.ToLower(l.Dirs[i].Name) < strings.ToLower(l.Dirs[j].Name) })
	return l, nil
}
