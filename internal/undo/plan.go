package undo

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// Host is a machine an undo runs on: its shell, and a file put there whole.
type Host interface {
	shell.Host
	PutFile(path string, data []byte, timeout time.Duration) error
}

// Service answers what undoing a turn would change and undoes it, at the person's word only.
type Service struct {
	Store   *Store
	Tracker *Tracker
	Host    func(machineID string) (Host, error)
	// Working is the agent at work in the folder, "" when none: an undo then would land under its feet
	Working func(machineID, dir string) string

	mu      sync.Mutex
	undoing map[string]bool // by machine|folder: one undo at a time
}

// ErrNotFound is a turn that is not kept.
var ErrNotFound = errors.New("no such turn")

// Refusal is why a turn cannot be undone now, and the files that stand in the way.
type Refusal struct {
	Why   string   `json:"why"`
	Files []string `json:"files,omitempty"`
}

func (r *Refusal) Error() string {
	if len(r.Files) == 0 {
		return r.Why
	}
	return r.Why + ": " + strings.Join(r.Files, ", ")
}

// Plan is what undoing a turn would do as the folder stands now.
type Plan struct {
	Turn        Turn     `json:"turn"`
	Refused     *Refusal `json:"refused,omitempty"`    // nothing can be undone now
	RevertWhy   string   `json:"revert_why,omitempty"` // why the turn's commits cannot be reverted now
	Pushed      []string `json:"pushed,omitempty"`     // the remote-tracking branches holding the turn's commits
	LeftAsIs    []string `json:"left_as_is,omitempty"` // without the reverts: files only the commits can put back
	Uncommitted int      `json:"uncommitted"`          // files put back without the reverts
	HeadMoved   bool     `json:"head_moved,omitempty"` // commits were made since the turn, on files it left alone
}

// Plan reads the folder and answers what an undo of the turn would do now, or why it cannot.
func (s *Service) Plan(machineID, id string) (Plan, error) {
	turn, ok := s.Store.Get(id)
	if !ok || turn.Machine != machineID {
		return Plan{}, ErrNotFound
	}
	plan := Plan{Turn: turn}
	refuse := func(why string, files ...string) (Plan, error) {
		plan.Refused = &Refusal{Why: why, Files: files}
		return plan, nil
	}
	switch {
	case turn.Undone != nil:
		return refuse("this turn was undone already")
	case turn.Why != "":
		return refuse(turn.Why)
	}
	if agent := s.working(turn); agent != "" {
		return refuse(agent + " is working in this folder: undo once it has stopped")
	}
	host, err := s.Host(machineID)
	if err != nil {
		return Plan{}, err
	}
	oldest := ""
	if len(turn.Commits) > 0 {
		oldest = turn.Commits[len(turn.Commits)-1].SHA
	}
	probe, err := read(host, turn.Dir, readOptions{base: turn.HeadEnd, staged: true, pushed: oldest})
	if errors.Is(err, ErrNotRepo) {
		return refuse("the folder is no longer a git checkout")
	}
	if err != nil {
		return Plan{}, err
	}
	plan.Pushed = probe.Pushed
	switch {
	case probe.Root != turn.Root:
		return refuse("the folder now belongs to another repository")
	case probe.Busy != "":
		return refuse("a " + probe.Busy + " is in progress in the folder: finish or abort it first")
	case probe.Branch != turn.Branch:
		return refuse(fmt.Sprintf("the folder is on %s now; the turn ended on %s", branchWords(probe.Branch), branchWords(turn.Branch)))
	case probe.Head != turn.HeadEnd && probe.Since != "linear":
		return refuse("the branch's history was rewritten since the turn (a reset, a rebase or an amend)")
	}
	plan.HeadMoved = probe.Head != turn.HeadEnd
	if changed := changedSince(turn, probe); len(changed) > 0 {
		return refuse("these files changed after the turn, so its changes no longer stand alone", changed...)
	}
	for _, file := range turn.Files {
		switch {
		case !file.Committed:
			plan.Uncommitted++
		case !file.DirtyThen && file.DirtyNow:
			plan.Uncommitted++
		case file.DirtyThen:
			plan.LeftAsIs = append(plan.LeftAsIs, file.Path)
		}
	}
	if len(turn.Commits) > 0 {
		plan.RevertWhy = revertWhy(turn, probe)
	}
	return plan, nil
}

func (s *Service) working(turn Turn) string {
	if s.Working != nil {
		if agent := s.Working(turn.Machine, turn.Dir); agent != "" {
			return agent
		}
	}
	if s.Tracker != nil && s.Tracker.Going(turn.Machine, turn.Dir) {
		return "an agent"
	}
	return ""
}

// changedSince names the turn's files that no longer hold what it left: changed in the working tree, or committed
// since by a commit after the turn's last.
func changedSince(turn Turn, probe Read) []string {
	var changed []string
	for _, file := range turn.Files {
		if _, committed := probe.Moved[file.Path]; committed {
			changed = append(changed, file.Path)
			continue
		}
		now := file.Head
		if entry, dirty := probe.Files[file.Path]; dirty {
			if entry.Old != file.Head {
				changed = append(changed, file.Path)
				continue
			}
			now = entry.New
		}
		if now != file.End {
			changed = append(changed, file.Path)
		}
	}
	sort.Strings(changed)
	return changed
}

// revertWhy is why the turn's commits cannot be reverted as the folder stands, "" when they can: git reverts on an
// index that holds nothing staged but what the undo itself puts back.
func revertWhy(turn Turn, probe Read) string {
	ours := map[string]bool{}
	for _, file := range turn.Files {
		ours[file.Path] = true
	}
	var staged []string
	for _, path := range probe.Staged {
		if !ours[path] {
			staged = append(staged, path)
		}
	}
	if len(staged) > 0 {
		sort.Strings(staged)
		if len(staged) > 5 {
			staged = append(staged[:5], fmt.Sprintf("%d more", len(staged)-5))
		}
		return "changes are staged in " + strings.Join(staged, ", ") + ": commit or unstage them before the commits are reverted"
	}
	return ""
}
