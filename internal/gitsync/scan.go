package gitsync

import (
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/state"
)

// The depth a folder is scanned to, as Manygit's setting: a repository that many folders down is found.
const (
	MinDepth     = 1
	MaxDepth     = 5
	DefaultDepth = 3
)

// MaxScan is how many repositories one scan lists; the rest are counted as more.
const MaxScan = 500

// scanFile keeps each machine's folder to scan, in the state dir.
const scanFile = "repos.json"

// pruned are the folders a scan never goes into, beside every hidden one: build output, dependencies, macOS's Library.
var pruned = []string{"node_modules", "vendor", "venv", "__pycache__", "dist", "build", "target", "site-packages", "Library"}

// Scan is a folder of the machine searched for repositories, and how deep.
type Scan struct {
	Folder string `json:"folder"`
	Depth  int    `json:"depth"`
}

// CheckScan is the scan as it is kept: the folder absolute (a ~/ one read against the machine's home), the depth
// within its bounds; a home folder, or a folder above one, is refused — too big to walk, and on a Mac walking it asks
// for Desktop, Documents and Downloads.
func CheckScan(home string, scan Scan) (Scan, error) {
	style := rpath.For(home)
	folder := strings.TrimSpace(scan.Folder)
	if folder == "~" || strings.HasPrefix(folder, "~/") {
		if home == "" {
			return Scan{}, errors.New("the machine's home is not known yet: name the folder in full")
		}
		folder = style.Join(home, strings.TrimPrefix(strings.TrimPrefix(folder, "~"), "/"))
	}
	if !style.IsAbs(folder) {
		return Scan{}, errors.New("the folder must be a full path, or one under ~/")
	}
	folder = style.Clean(folder)
	if unsearched(style, home, folder) {
		return Scan{}, errors.New("a home folder, or a folder above one, is too big to scan: pick the folder that holds your repositories")
	}
	if scan.Depth < MinDepth || scan.Depth > MaxDepth {
		return Scan{}, errors.New("the depth goes from 1 to 5")
	}
	return Scan{Folder: folder, Depth: scan.Depth}, nil
}

// unsearched is the root, a home of the usual places (/root, /home/<name>, /Users/<name>, C:\Users\<name>), or this
// machine's home, or any folder above one of them.
func unsearched(style rpath.Style, home, folder string) bool {
	if home != "" && style.Within(folder, home) {
		return true
	}
	parts := strings.FieldsFunc(style.Slash(folder), func(r rune) bool { return r == '/' })
	if style.Windows && len(parts) > 0 {
		parts = parts[1:] // the drive
	}
	switch len(parts) {
	case 0:
		return true
	case 1:
		return parts[0] == "root" || parts[0] == "home" || strings.EqualFold(parts[0], "Users")
	case 2:
		return parts[0] == "home" || strings.EqualFold(parts[0], "Users")
	}
	return false
}

// Scans is each machine's folder to scan, kept in the state dir.
type Scans struct {
	dir string

	mu    sync.Mutex
	scans map[string]Scan
}

// OpenScans reads the scans kept in dir; a damaged file is none, said once.
func OpenScans(dir string) *Scans {
	scans := &Scans{dir: dir, scans: map[string]Scan{}}
	if err := state.LoadJSON(dir, scanFile, &scans.scans); err != nil {
		log.Printf("%s is unreadable (%v): no folder is scanned for repositories", scanFile, err)
		scans.scans = map[string]Scan{}
	}
	return scans
}

// Of is the machine's scan, false for none.
func (s *Scans) Of(machineID string) (Scan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scan, ok := s.scans[machineID]
	return scan, ok
}

// Set keeps the machine's scan; an empty folder stops it.
func (s *Scans) Set(machineID string, scan Scan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scan.Folder == "" {
		delete(s.scans, machineID)
	} else {
		s.scans[machineID] = scan
	}
	return state.SaveJSON(s.dir, scanFile, s.scans)
}

// Forget drops a removed machine's scan.
func (s *Scans) Forget(machineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.scans[machineID]; !ok {
		return
	}
	delete(s.scans, machineID)
	if err := state.SaveJSON(s.dir, scanFile, s.scans); err != nil {
		log.Printf("%s: %v", scanFile, err)
	}
}
