package opinion

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

const file = "opinions.json"

// The states of an opinion: asked and not answered yet, answered and read, answered in a form Parse could not read,
// stopped on calls Claude Code denied without asking before any answer, and the reviewer's pane gone before it
// answered.
const (
	Running    = "running"
	Done       = "done"
	Unreadable = "unreadable"
	Stopped    = "stopped"
	Gone       = "gone"
)

// Comment is a finding kept for the person, numbered within its opinion.
type Comment struct {
	ID int `json:"id"`
	Finding
	Dismissed bool `json:"dismissed,omitempty"`
}

// Opinion is one second opinion on a folder: who was asked, on what, where it runs, and what it said.
type Opinion struct {
	Ref       string            `json:"ref"` // named in the request, so the reviewer's conversation is told from an earlier one in the folder
	Machine   string            `json:"machine"`
	Dir       string            `json:"dir"`
	Agent     string            `json:"agent"`
	Model     string            `json:"model,omitempty"`
	Workspace string            `json:"workspace"`
	Tab       string            `json:"tab"`
	Pane      string            `json:"pane"`
	Head      string            `json:"head,omitempty"`
	Blobs     map[string]string `json:"blobs,omitempty"` // the blob of every uncommitted file the reviewer was shown
	Asked     time.Time         `json:"asked"`
	Answered  time.Time         `json:"answered,omitzero"`
	State     string            `json:"state"`
	Note      string            `json:"note,omitempty"` // why there is nothing to show: unreadable, gone, the tab kept
	Summary   string            `json:"summary,omitempty"`
	Comments  []Comment         `json:"comments"`
	Close     bool              `json:"close"` // the reviewer's tab closes once it has answered
	// Moved is the files of the comments that changed since the reviewer was asked: their lines may have moved. Worked
	// out when the opinion is read for the page, never kept.
	Moved []string `json:"moved,omitempty"`
}

// listening says whether the opinion still takes an answer from its reviewer: one asked and not answered, one answered
// in a form that could not be read — a question in plain words, say — or one stopped on denied calls, whose reviewer
// may answer once the person has taken it on in its tab
func (op Opinion) listening() bool {
	return op.State == Running || op.State == Unreadable || op.State == Stopped
}

// Open is how many comments are neither added to the person's review nor dismissed.
func (op Opinion) Open() int {
	n := 0
	for _, c := range op.Comments {
		if !c.Dismissed {
			n++
		}
	}
	return n
}

// ErrNone is a folder with no second opinion.
var ErrNone = errors.New("no second opinion on this folder")

// Store keeps the newest opinion of every machine|folder in the state dir.
type Store struct {
	dir string
	mu  sync.Mutex
	ops map[string]Opinion
	now func() time.Time
}

func key(machine, dir string) string { return machine + "|" + dir }

// NewStore reads what the state dir kept; a damaged file is an empty store, said once.
func NewStore(dir string) *Store {
	store := &Store{dir: dir, ops: map[string]Opinion{}, now: time.Now}
	if err := state.LoadJSON(dir, file, &store.ops); err != nil {
		log.Printf("%s is unreadable (%v): no second opinion is kept from before", file, err)
		store.ops = map[string]Opinion{}
	}
	return store
}

func (store *Store) save() {
	if err := state.SaveJSON(store.dir, file, store.ops); err != nil {
		log.Printf("the second opinions could not be kept: %v", err)
	}
}

// Begin keeps a new opinion as running, in place of the folder's last.
func (store *Store) Begin(op Opinion) Opinion {
	op.State, op.Comments, op.Asked, op.Ref = Running, []Comment{}, store.now(), state.NewID()
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ops[key(op.Machine, op.Dir)] = op
	store.save()
	return op
}

// Get is the folder's opinion.
func (store *Store) Get(machine, dir string) (Opinion, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	op, ok := store.ops[key(machine, dir)]
	return op, ok
}

// Answer reads what the reviewer in that pane answered into the opinion it was asked for; false when the pane is no
// running reviewer's. A readable answer is the opinion's comments; another is noted, the reviewer's tab kept for the
// person to read.
func (store *Store) Answer(machine, pane, text string) (Opinion, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for k, op := range store.ops {
		if op.Machine != machine || op.Pane != pane || !op.listening() {
			continue
		}
		answer := Parse(text, op.Dir)
		op.Answered, op.Summary = store.now(), answer.Summary
		if answer.Readable() {
			op.State, op.Note = Done, ""
			op.Comments = make([]Comment, 0, len(answer.Findings))
			for i, finding := range answer.Findings {
				op.Comments = append(op.Comments, Comment{ID: i + 1, Finding: finding})
			}
		} else {
			op.State, op.Note = Unreadable, "the reviewer did not answer in the path:line form; its answer is in its tab"
		}
		store.ops[k] = op
		store.save()
		return op, true
	}
	return Opinion{}, false
}

// Stop notes that the reviewer in that pane stopped on calls Claude Code denied without asking, before it answered:
// no answer is on its way, the tab stays for the person to see them, and an answer the reviewer gives later is heard.
func (store *Store) Stop(machine, pane string) (Opinion, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for k, op := range store.ops {
		if op.Machine != machine || op.Pane != pane || !op.listening() {
			continue
		}
		if op.State == Stopped {
			return op, true
		}
		op.State, op.Note, op.Answered = Stopped, "the reviewer stopped on calls it was denied without asking; they are listed in its tab", store.now()
		store.ops[k] = op
		store.save()
		return op, true
	}
	return Opinion{}, false
}

// Waiting says whether an opinion waits on that pane's answer.
func (store *Store) Waiting(machine, pane string) bool {
	_, ok := store.waitingOn(machine, pane)
	return ok
}

// waitingOn is the opinion that waits on that pane's answer.
func (store *Store) waitingOn(machine, pane string) (Opinion, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, op := range store.ops {
		if op.Machine == machine && op.Pane == pane && op.listening() {
			return op, true
		}
	}
	return Opinion{}, false
}

// Reviewer says whether the pane is a second opinion's reviewer: its answers are the opinion's, never the folder's
// own agent saying it is ready for another look.
func (store *Store) Reviewer(machine, pane string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, op := range store.ops {
		if op.Machine == machine && op.Pane == pane {
			return true
		}
	}
	return false
}

// Running is the folder's opinion while its reviewer has not answered yet.
func (store *Store) Running(machine, dir string) (Opinion, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	op, ok := store.ops[key(machine, dir)]
	return op, ok && op.State == Running
}

// runningOn is every opinion of the machine whose reviewer has not answered yet.
func (store *Store) runningOn(machine string) []Opinion {
	store.mu.Lock()
	defer store.mu.Unlock()
	var list []Opinion
	for _, op := range store.ops {
		if op.Machine == machine && op.State == Running {
			list = append(list, op)
		}
	}
	return list
}

// Gone notes that a running reviewer's pane closed before it answered.
func (store *Store) Gone(machine, pane string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	for k, op := range store.ops {
		if op.Machine == machine && op.Pane == pane && op.State == Running {
			op.State, op.Note = Gone, "the reviewer's tab closed before it answered"
			store.ops[k] = op
			store.save()
			return true
		}
	}
	return false
}

// Fail notes that the opinion asked of that pane could not be asked: the agent did not start, or the request never
// reached it.
func (store *Store) Fail(machine, pane, why string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for k, op := range store.ops {
		if op.Machine == machine && op.Pane == pane && op.State == Running {
			op.State, op.Note = Gone, why
			store.ops[k] = op
			store.save()
			return
		}
	}
}

// Moved is the files of the opinion's open comments whose blob is no longer the one the reviewer was shown: the
// blobs of the folder's uncommitted files now. A file no longer uncommitted is the lens's to leave out, not moved.
func Moved(op Opinion, now map[string]string) []string {
	seen := map[string]bool{}
	var moved []string
	for _, c := range op.Comments {
		if c.Dismissed || seen[c.Path] {
			continue
		}
		seen[c.Path] = true
		was, asked := op.Blobs[c.Path]
		is, there := now[c.Path]
		if there && (!asked || was != is) {
			moved = append(moved, c.Path)
		}
	}
	return moved
}

// Dismiss sets the comments with those ids aside (all of them for none given): added to the person's own review, or
// not wanted.
func (store *Store) Dismiss(machine, dir string, ids []int) (Opinion, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	op, ok := store.ops[key(machine, dir)]
	if !ok {
		return Opinion{}, ErrNone
	}
	wanted := map[int]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	op.Comments = append([]Comment(nil), op.Comments...) // the copies Get handed out are read outside the lock
	for i := range op.Comments {
		if len(ids) == 0 || wanted[op.Comments[i].ID] {
			op.Comments[i].Dismissed = true
		}
	}
	store.ops[key(machine, dir)] = op
	store.save()
	return op, nil
}

// Forget drops the folder's opinion.
func (store *Store) Forget(machine, dir string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.ops[key(machine, dir)]; ok {
		delete(store.ops, key(machine, dir))
		store.save()
	}
}

// ForgetMachine drops every opinion of a machine that was removed.
func (store *Store) ForgetMachine(machine string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	changed := false
	for k, op := range store.ops {
		if op.Machine == machine {
			delete(store.ops, k)
			changed = true
		}
	}
	if changed {
		store.save()
	}
}
