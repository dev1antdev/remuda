// Package filetree is the project tree of the files rail: one folder of a workspace listed at a time, folders first,
// with git's word on each entry — new, added, modified, deleted, in conflict, ignored, a folder holding changes —
// from one `git status` per checkout, read once and kept a few seconds while the tree's folders are read against it.
// Everything runs on the folder's machine through its shell; nothing is written there.
package filetree

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const (
	PerFolder    = 500             // a folder lists this many entries at most
	StatusFor    = 4 * time.Second // a checkout's git status stands this long for the folders read against it
	statusKept   = time.Minute     // a status nobody asked for in this long is dropped
	listTimeout  = 20 * time.Second
	statusLimit  = 4 << 20 // bytes of git's answer read at most
	statusWindow = 20 * time.Second
)

// Entry is one name in a listed folder.
type Entry struct {
	Name    string `json:"name"`
	Dir     bool   `json:"dir,omitempty"`
	Link    bool   `json:"link,omitempty"`    // a symlink, followed for Dir
	Mark    string `json:"mark,omitempty"`    // A added, M modified, D deleted, U in conflict, ? new and not yet in git
	Ignored bool   `json:"ignored,omitempty"` // git ignores it, or a folder it is in
	Changes bool   `json:"changes,omitempty"` // a folder holding a changed file
	Gone    bool   `json:"gone,omitempty"`    // deleted from the working tree, listed from git's word alone
}

// Listing is one folder of the tree: Path relative to the tree's root ("" the root), its entries folders first.
type Listing struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
	More    bool    `json:"more,omitempty"` // the folder holds more than PerFolder
	Repo    bool    `json:"repo"`           // the tree is inside a git checkout
	Git     string  `json:"git,omitempty"`  // why git could not read it
}

// ErrOutside refuses a path that climbs out of the tree's root.
var ErrOutside = errors.New("the path leaves the workspace folder")

// Service lists folders; its zero value is ready to use.
type Service struct {
	Now func() time.Time

	mu    sync.Mutex
	reads map[string]*statusRead
}

type statusRead struct {
	done   chan struct{}
	at     time.Time
	status Status
	err    error
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Folder is the absolute folder a listing of rel under root reads, in the machine's spelling.
func Folder(style rpath.Style, root, rel string) (string, error) {
	if !rpath.Absolute(root) {
		return "", fmt.Errorf("an absolute folder is required")
	}
	if rel == "" {
		return style.Clean(root), nil
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsRune(part, 0) || style.Windows && strings.Contains(part, `\`) {
			return "", ErrOutside
		}
	}
	return style.Join(root, rel), nil
}

// List reads one folder of the tree rooted at root on the host, with git's marks.
func (s *Service) List(host shell.Host, root, rel string) (Listing, error) {
	style := rpath.For(host.Home())
	folder, err := Folder(style, root, rel)
	if err != nil {
		return Listing{}, err
	}
	out, err := shell.Text(host, listScript(style.Slash(folder)), listTimeout)
	if err != nil {
		return Listing{}, err
	}
	entries, more, err := ParseListing(out)
	if err != nil {
		return Listing{}, err
	}
	status, err := s.status(host, style.Slash(style.Clean(root)))
	listing := Listing{Path: rel, More: more, Entries: entries}
	if err != nil {
		listing.Git = err.Error()
	} else {
		listing.Repo = status.Repo
		listing.Git = status.Err
		listing.Entries = status.Apply(rel, entries, more)
	}
	sortEntries(listing.Entries)
	return listing, nil
}

// status is the checkout's git status for the tree at root, read once for every folder asked within StatusFor.
func (s *Service) status(host shell.Host, root string) (Status, error) {
	key := host.ID() + "|" + root
	now := s.now()
	s.mu.Lock()
	if s.reads == nil {
		s.reads = map[string]*statusRead{}
	}
	for k, r := range s.reads {
		if finished(r) && now.Sub(r.at) > statusKept {
			delete(s.reads, k)
		}
	}
	if r := s.reads[key]; r != nil && (!finished(r) || now.Sub(r.at) < StatusFor) {
		s.mu.Unlock()
		<-r.done
		return r.status, r.err
	}
	r := &statusRead{done: make(chan struct{})}
	s.reads[key] = r
	s.mu.Unlock()
	status, err := readStatus(host, root)
	s.mu.Lock()
	r.status, r.err, r.at = status, err, s.now()
	s.mu.Unlock()
	close(r.done)
	return status, err
}

func finished(r *statusRead) bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

func readStatus(host shell.Host, root string) (Status, error) {
	out, err := shell.Text(host, statusScript(root), statusWindow)
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(out), nil
}

// listScript prints the folder's folders, then its files, each as its kind, US and its name, ended by NUL: d a
// folder, f a file, l before either a symlink; "more" ends a folder holding past PerFolder. .git is never listed.
func listScript(folder string) string {
	return `cd ` + script.Quote(folder) + ` 2>/dev/null || { printf 'ERR\037not a folder: %s\n' ` + script.Quote(folder) + `; exit 0; }
n=0; more=0
list() {
  for p in * .[!.]* ..?*; do
    [ -e "$p" ] || [ -L "$p" ] || continue
    [ "$p" = .git ] && continue
    if [ -d "$p" ]; then [ "$1" = d ] || continue; else [ "$1" = f ] || continue; fi
    if [ "$n" -ge ` + fmt.Sprint(PerFolder) + ` ]; then more=1; return; fi
    n=$((n+1)); k=$1; [ -L "$p" ] && k="l$1"
    printf '%s\037%s\000' "$k" "$p"
  done
}
list d; [ "$more" = 1 ] || list f; [ "$more" = 1 ] && printf 'more\000'; exit 0`
}

// ParseListing reads listScript's answer.
func ParseListing(out string) ([]Entry, bool, error) {
	if rest, ok := strings.CutPrefix(out, "ERR"+script.US); ok {
		return nil, false, errors.New(strings.TrimSpace(rest))
	}
	entries := []Entry{}
	more := false
	for _, record := range strings.Split(out, "\x00") {
		if record == "more" {
			more = true
			continue
		}
		kind, name, ok := strings.Cut(record, script.US)
		if !ok || name == "" {
			continue
		}
		entries = append(entries, Entry{Name: name, Dir: strings.HasSuffix(kind, "d"), Link: strings.HasPrefix(kind, "l")})
	}
	return entries, more, nil
}

// sortEntries puts folders first, each kind by name the way a file browser reads, case aside.
func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Name < b.Name
	})
}
