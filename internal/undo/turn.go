package undo

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// MaxDelta is the most a turn's kept patches may weigh; a turn past it is listed but cannot be undone.
var MaxDelta = 16 << 20

// What a turn did to a file, as the undo puts it back
const (
	WasAdded     = "added"     // new in the turn: the undo removes it
	WasDeleted   = "deleted"   // removed in the turn: the undo brings it back
	WasChanged   = "changed"   // its content moved: the undo puts it back
	WasMode      = "mode"      // only its mode moved
	WasCommitted = "committed" // the working tree holds it as before, but the turn's commits carry a change of it
)

// File is one path the turn touched or committed, with what it held when the turn began and what the turn left.
type File struct {
	Path      string `json:"path"`
	Was       string `json:"was"`
	Committed bool   `json:"committed,omitempty"` // the turn's commits changed it
	Binary    bool   `json:"binary,omitempty"`
	Start     Side   `json:"start"`    // the working tree's when the turn began
	End       Side   `json:"end"`      // the working tree's when it ended
	Head      Side   `json:"head"`     // HEAD's when it ended
	DirtyThen bool   `json:"dirty_at"` // uncommitted when the turn began: the start's patch carries it
	DirtyNow  bool   `json:"dirty_to"` // uncommitted when the turn ended: the end's patch carries it
}

// Turn is one agent's turn in a folder: from herdr's working to its rest, what it changed there and how to put it back.
type Turn struct {
	ID        string    `json:"id"`
	Machine   string    `json:"machine"`
	Dir       string    `json:"dir"`
	Root      string    `json:"root"`
	Pane      string    `json:"pane"`
	Agent     string    `json:"agent,omitempty"`
	Started   time.Time `json:"started"`
	Ended     time.Time `json:"ended"`
	Branch    string    `json:"branch,omitempty"`
	HeadStart string    `json:"head_start"`
	HeadEnd   string    `json:"head_end"`
	Files     []File    `json:"files"`
	Commits   []Commit  `json:"commits,omitempty"` // newest first
	Size      int       `json:"size"`              // bytes of the patches kept
	Why       string    `json:"why,omitempty"`     // why it can never be undone: too big, cut, a branch switched, history rewritten
	Shared    bool      `json:"shared,omitempty"`  // another agent worked in the folder at the same time: its changes are in it too
	Undone    *Undone   `json:"undone,omitempty"`
}

// Listed is a turn as a folder's list carries it: its files counted, the files themselves left to the plan, since a
// chat on screen reads the list every few seconds.
type Listed struct {
	Turn
	Files int `json:"files"`
}

// Listing is the turns as a folder's list carries them.
func Listing(turns []Turn) []Listed {
	out := make([]Listed, 0, len(turns))
	for _, turn := range turns {
		out = append(out, Listed{Turn: turn, Files: len(turn.Files)})
	}
	return out
}

// Undone is how a turn was undone.
type Undone struct {
	At       time.Time `json:"at"`
	Files    int       `json:"files"`
	Reverted []string  `json:"reverted,omitempty"` // the revert commits made, newest last
}

// Delta compares a turn's two checkpoints: every path whose content or mode moved, every path its commits changed,
// and the sections of the two patches the undo needs — the end's reverse for what was uncommitted at the end, the
// start's for what was uncommitted at the start. A turn that moved nothing comes back with no files.
func Delta(start, end Read) (Turn, string, string) {
	turn := Turn{Root: end.Root, Branch: end.Branch, HeadStart: start.Head, HeadEnd: end.Head}
	moved := map[string]Entry{}
	if start.Head != end.Head {
		switch {
		case start.Branch != end.Branch:
			turn.Why = fmt.Sprintf("the turn switched the folder from %s to %s", branchWords(start.Branch), branchWords(end.Branch))
		case end.Since != "linear":
			turn.Why = "the turn rewrote the branch's history (a rebase, a reset or an amend): its commits cannot be reverted"
		case len(end.Made) > MaxCommits:
			turn.Why = fmt.Sprintf("the turn made more than %d commits", MaxCommits)
		case broughtIn(end.Made, start.At):
			turn.Why = "the turn brought in commits made before it began (a pull or a fast-forward): remuda cannot tell them from its own"
		default:
			moved = end.Moved
			turn.Commits = end.Made
		}
	}
	if turn.Why == "" && (start.Busy != "" || end.Busy != "") {
		turn.Why = "a " + firstOf(end.Busy, start.Busy) + " was in progress during the turn"
	}
	paths := map[string]bool{}
	for _, set := range []map[string]Entry{start.Files, end.Files, moved} {
		for path := range set {
			paths[path] = true
		}
	}
	startSections := sections(start.Patch, start.Cut)
	endSections := sections(end.Patch, end.Cut)
	var startPatch, endPatch strings.Builder
	missing := 0
	for _, path := range sortedKeys(paths) {
		file, relevant := fileOf(path, start.Files, end.Files, moved)
		if !relevant {
			continue
		}
		if file.DirtyThen {
			section, ok := startSections[path]
			if !ok {
				missing++
			}
			startPatch.WriteString(section)
			file.Binary = file.Binary || binary(section)
		}
		if file.DirtyNow {
			section, ok := endSections[path]
			if !ok {
				missing++
			}
			endPatch.WriteString(section)
			file.Binary = file.Binary || binary(section)
		}
		turn.Files = append(turn.Files, file)
	}
	turn.Size = startPatch.Len() + endPatch.Len()
	switch {
	case turn.Why != "":
	case missing > 0:
		turn.Why = fmt.Sprintf("the folder's uncommitted work was too big to keep whole (over %s)", sizeWords(MaxCheckpoint))
	case turn.Size > MaxDelta:
		turn.Why = fmt.Sprintf("the turn's changes weigh %s, past the %s an undo keeps", sizeWords(turn.Size), sizeWords(MaxDelta))
	}
	if turn.Why != "" {
		turn.Size = 0
		return turn, "", ""
	}
	return turn, startPatch.String(), endPatch.String()
}

// fileOf is one path's sides at the turn's start and end, and whether the turn touched it or committed it.
func fileOf(path string, startFiles, endFiles, moved map[string]Entry) (File, bool) {
	file := File{Path: path}
	startEntry, dirtyThen := startFiles[path]
	endEntry, dirtyNow := endFiles[path]
	movedEntry, committed := moved[path]
	file.DirtyThen, file.DirtyNow, file.Committed = dirtyThen, dirtyNow, committed
	var headStart, headEnd Side
	switch {
	case dirtyThen:
		headStart = startEntry.Old
	case committed:
		headStart = movedEntry.Old
	default:
		headStart = endEntry.Old
	}
	switch {
	case dirtyNow:
		headEnd = endEntry.Old
	case committed:
		headEnd = movedEntry.New
	default:
		headEnd = headStart
	}
	file.Start, file.End, file.Head = headStart, headEnd, headEnd
	if dirtyThen {
		file.Start = startEntry.New
	}
	if dirtyNow {
		file.End = endEntry.New
	}
	switch {
	case file.Start == file.End && committed:
		file.Was = WasCommitted
	case file.Start == file.End:
		return file, false
	case file.Start.Absent():
		file.Was = WasAdded
	case file.End.Absent():
		file.Was = WasDeleted
	case file.Start.Blob == file.End.Blob:
		file.Was = WasMode
	default:
		file.Was = WasChanged
	}
	return file, true
}

// broughtIn says a commit of the range was committed before the turn began, by the machine's own clock at its start:
// one the turn fetched rather than made, which a revert of the turn must not take back.
func broughtIn(commits []Commit, began int64) bool {
	if began == 0 {
		return false
	}
	for _, commit := range commits {
		if commit.When < began {
			return true
		}
	}
	return false
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func branchWords(branch string) string {
	if branch == "" {
		return "a detached HEAD"
	}
	return branch
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// sizeWords is a byte count as a person reads it: "812 B", "14 KB", "3.2 MB".
func sizeWords(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%d KB", n/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
