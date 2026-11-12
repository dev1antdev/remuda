// Package collisions is the collision radar: agents in different checkouts of one repository — worktrees of one
// machine, clones on several — changing the same files, told before the merge says so. Each checkout's work is read
// against the repository's main branch (script.go), the works of one repository compared two by two (this file), and
// the overlaps kept for the roster, with one push per new overlap of the same lines (service.go).
package collisions

import (
	"sort"
	"strings"

	"github.com/dev1antdev/remuda/internal/teleport"
)

// Workspace is one workspace the radar weighs: where it is, the repository it belongs to, and how its agents stand.
type Workspace struct {
	Machine string
	ID      string // herdr's workspace id
	Label   string
	Dir     string
	Branch  string
	Repo    string // the key every checkout of one repository shares (RepoKey)
	Pane    string // its first agent pane, for a push to open
	Agents  bool   // an agent runs in it
	Working bool   // one of its agents is at work
	PR      bool   // one of its agents named a pull request that is still open
	Changed int    // uncommitted files, as the review sweep counted them
}

// active says whether a workspace counts for the radar: an agent runs there, or work waits uncommitted.
func (w Workspace) active() bool { return w.Repo != "" && (w.Agents || w.Changed > 0) }

// RepoKey is what every checkout of one repository shares: its remote's address, which reads the same over ssh and
// https, else — a repository with no remote — its main folder on the machine; "" outside git.
func RepoKey(machineID, root, remote string) string {
	if key := teleport.RemoteKey(remote); key != "" {
		return "remote " + key
	}
	if root != "" {
		return "root " + machineID + "|" + root
	}
	return ""
}

// Groups are the repositories two or more active workspaces in different folders work on, each its workspaces in
// folder order; a folder two workspaces share is one checkout. Only these are worth reading.
func Groups(workspaces []Workspace) [][]Workspace {
	byRepo := map[string][]Workspace{}
	for _, ws := range workspaces {
		if ws.active() {
			byRepo[ws.Repo] = append(byRepo[ws.Repo], ws)
		}
	}
	var groups [][]Workspace
	for _, members := range byRepo {
		folders := map[string]bool{}
		for _, ws := range members {
			folders[ws.Machine+"|"+ws.Dir] = true
		}
		if len(folders) < 2 {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool {
			return members[i].Machine+"|"+members[i].Dir < members[j].Machine+"|"+members[j].Dir
		})
		groups = append(groups, members)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0].Repo < groups[j][0].Repo })
	return groups
}

// Hunk is one change's place in the base, as git's -U0 hunk header names it: Count lines from Start, or — Count 0 —
// lines inserted after line Start; Size is the lines it removes and adds.
type Hunk struct {
	Start, Count, Size int
}

// Change is one file a work changed against its base: the blob it has now (Gone for one deleted, "" when git names
// none: an untracked file), the hunks, or Whole for a file changed as a whole — a binary one, a new untracked one.
type Change struct {
	Blob  string
	Hunks []Hunk
	Whole bool
}

// Work is one checkout's work against its repository's main branch: its branch's own commits and what is still
// uncommitted, by file of the repository.
type Work struct {
	Machine string
	Dir     string // the folder the workspace stands on
	Top     string // the checkout's top folder: two folders inside one checkout are one work
	Main    string // the branch it was read against, "origin/main"
	Base    string // the merge base with it
	Files   map[string]Change
	Lines   int  // lines removed and added in all
	Cut     bool // more changes than one reading carries: the rest not read
}

// Overlap is one file two works both change: whether their hunks touch the same lines (or lines next to them, which
// a merge counts as one conflict too), and where, in the base's lines; From 0 is the whole file.
type Overlap struct {
	Path  string `json:"path"`
	Lines bool   `json:"lines"`
	From  int    `json:"from,omitempty"`
	To    int    `json:"to,omitempty"`
}

// Peer is the workspace a collision is with.
type Peer struct {
	Machine   string `json:"machine"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Branch    string `json:"branch,omitempty"`
}

// MaxFiles is how many files a collision names; the rest are counted.
const MaxFiles = 20

// Collision is one work's overlap with another's, from its own side: the files, whether some share lines, and which of
// the two should land first, and why.
type Collision struct {
	With  Peer      `json:"with"`
	Files []Overlap `json:"files"`
	More  int       `json:"more,omitempty"`
	Lines bool      `json:"lines"` // some file's hunks touch the same lines
	First bool      `json:"first"` // this work is the one to land first
	Why   string    `json:"why"`   // pr | rest | smaller | name: what decided the order
	Main  string    `json:"main"`  // the branch both were read against
}

// The reasons one work goes first.
const (
	WhyPR      = "pr"      // it has a pull request open and the other has none
	WhyRest    = "rest"    // its agents have finished and the other's still work
	WhySmaller = "smaller" // its change is the smaller one
	WhyName    = "name"    // nothing tells them apart: by name
)

// halves are a hunk's place in half-lines, so an insertion between two lines sits between them: lines Start..End are
// [2·Start, 2·End], an insertion after line Start is 2·Start+1.
func (h Hunk) halves() (int, int) {
	if h.Count == 0 {
		return 2*h.Start + 1, 2*h.Start + 1
	}
	return 2 * h.Start, 2 * (h.Start + h.Count - 1)
}

func (h Hunk) first() int { return max(1, h.Start) }
func (h Hunk) last() int  { return max(1, h.Start+max(h.Count, 1)-1) }

// overlap compares one file's change in two works: nothing when both left it as the same blob (a merge of the same
// change is clean), the same lines when their hunks touch — the same lines, or lines side by side — or when either
// changed it whole, else the file alone.
func overlap(path string, a, b Change) (Overlap, bool) {
	if a.Blob != "" && a.Blob == b.Blob {
		return Overlap{}, false
	}
	if a.Whole || b.Whole {
		return Overlap{Path: path, Lines: true}, true
	}
	for _, ha := range a.Hunks {
		aFrom, aTo := ha.halves()
		for _, hb := range b.Hunks {
			bFrom, bTo := hb.halves()
			if aFrom <= bTo+2 && bFrom <= aTo+2 {
				return Overlap{Path: path, Lines: true, From: min(ha.first(), hb.first()), To: max(ha.last(), hb.last())}, true
			}
		}
	}
	return Overlap{Path: path}, true
}

// Compare is what two works of one repository both change, the files whose lines touch first, then by path.
func Compare(a, b Work) []Overlap {
	var out []Overlap
	for path, change := range a.Files {
		if other, ok := b.Files[path]; ok {
			if o, ok := overlap(path, change, other); ok {
				out = append(out, o)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Lines != out[j].Lines {
			return out[i].Lines
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Standing is what decides the order of two works: a pull request open, the agents at work, the change's size, the name.
type Standing struct {
	PR      bool
	Working bool
	Lines   int
	Name    string
}

// Order says whether a goes before b, and why: the one with a pull request open first, else the one whose agents have
// finished, else the smaller change, else by name — the one closer to done lands first and the other rebases on it.
func Order(a, b Standing) (bool, string) {
	switch {
	case a.PR != b.PR:
		return a.PR, WhyPR
	case a.Working != b.Working:
		return !a.Working, WhyRest
	case a.Lines != b.Lines:
		return a.Lines < b.Lines, WhySmaller
	}
	return strings.ToLower(a.Name) <= strings.ToLower(b.Name), WhyName
}

// sides are the two collisions one overlap makes: a's side, then b's.
func sides(a, b Work, peerA, peerB Peer, standA, standB Standing, files []Overlap) (Collision, Collision) {
	lines := len(files) > 0 && files[0].Lines
	more := 0
	if len(files) > MaxFiles {
		more, files = len(files)-MaxFiles, files[:MaxFiles]
	}
	aFirst, why := Order(standA, standB)
	main := a.Main
	if main == "" {
		main = b.Main
	}
	return Collision{With: peerB, Files: files, More: more, Lines: lines, First: aFirst, Why: why, Main: main},
		Collision{With: peerA, Files: files, More: more, Lines: lines, First: !aFirst, Why: why, Main: main}
}
