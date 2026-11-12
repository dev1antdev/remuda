// Package triggers acts on what a pane prints. herdr itself watches each pane's rows for a match — the watches ride
// the machine's event stream (internal/watch) — and this package says which watches to keep and what a match means:
// the built-in ones (a dev server announcing its address, offered as a preview; tests that failed, one push per
// episode) and the person's own rules (a pattern, everywhere or in a folder, and an action), kept in triggers.json.
package triggers

import (
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

const file = "triggers.json"

// The actions a rule can take.
const (
	ActionPush    = "push"    // a notification, and its row in the activity
	ActionPreview = "preview" // the port the line names offered as a preview
	ActionTask    = "task"    // the task whose agent runs in the pane moved to a column
	ActionFeed    = "feed"    // a row in the activity, nothing more
)

// Limits on what the person keeps.
const (
	MaxRules   = 50
	MaxPattern = 400
	MaxName    = 80
)

// Rule is one of the person's triggers: where it looks, what it looks for, and what it does.
type Rule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Machine string `json:"machine,omitempty"` // "" for every machine
	Dir     string `json:"dir,omitempty"`     // a folder of the machine and everything under it; "" for every folder
	Match   string `json:"match"`             // substring | regex
	Pattern string `json:"pattern"`
	Action  string `json:"action"`
	Column  string `json:"column,omitempty"` // the column a task action moves the task to, by its title
	Enabled bool   `json:"enabled"`
}

// Check trims a rule and says what is wrong with it; herdr reads the regex in Rust's dialect, so what Go's reads and
// Rust's does not, or reads otherwise, is refused here, before it could refuse the person's watches on a machine.
func Check(r *Rule) error {
	r.Name, r.Pattern, r.Column = strings.TrimSpace(r.Name), strings.TrimRight(r.Pattern, "\r\n"), strings.TrimSpace(r.Column)
	r.Machine, r.Dir = strings.TrimSpace(r.Machine), strings.TrimSpace(r.Dir)
	switch {
	case r.Name == "":
		return errors.New("a trigger needs a name")
	case utf8.RuneCountInString(r.Name) > MaxName:
		return fmt.Errorf("a trigger's name is %d characters at most", MaxName)
	case strings.TrimSpace(r.Pattern) == "":
		return fmt.Errorf("trigger %q looks for nothing", r.Name)
	case len(r.Pattern) > MaxPattern:
		return fmt.Errorf("trigger %q: a pattern is %d bytes at most", r.Name, MaxPattern)
	case strings.ContainsAny(r.Pattern, "\r\n"):
		return fmt.Errorf("trigger %q: herdr matches one line at a time, so a pattern holds no line break", r.Name)
	}
	switch r.Match {
	case "substring":
	case "regex":
		if err := checkRegex(r.Pattern); err != nil {
			return fmt.Errorf("trigger %q: %w", r.Name, err)
		}
	default:
		return fmt.Errorf("trigger %q: match by substring or regex", r.Name)
	}
	switch r.Action {
	case ActionPush, ActionPreview, ActionFeed:
		r.Column = ""
	case ActionTask:
		if r.Column == "" {
			return fmt.Errorf("trigger %q: name the column the task moves to", r.Name)
		}
		if utf8.RuneCountInString(r.Column) > MaxName {
			return fmt.Errorf("trigger %q: a column's name is %d characters at most", r.Name, MaxName)
		}
	default:
		return fmt.Errorf("trigger %q: unknown action %q", r.Name, r.Action)
	}
	if r.Dir != "" {
		if !strings.HasPrefix(r.Dir, "/") {
			return fmt.Errorf("trigger %q: the folder is an absolute path", r.Name)
		}
		if r.Machine == "" {
			return fmt.Errorf("trigger %q: a folder belongs to a machine", r.Name)
		}
		r.Dir = path.Clean(r.Dir)
	}
	return nil
}

// checkRegex is the pattern as herdr and Go both read it, and never one every line matches.
func checkRegex(pattern string) error {
	re, err := herdr.CompileRegex(pattern)
	if err != nil {
		return err
	}
	if re.MatchString("") {
		return errors.New("the regex matches an empty line, so every line")
	}
	return nil
}

// Store is the person's rules, replaced whole.
type Store struct {
	dir string

	mu    sync.Mutex
	rules []Rule
}

// Open reads triggers.json from the state dir; a damaged file is no rules, said once.
func Open(dir string) *Store {
	st := &Store{dir: dir}
	if err := state.LoadJSON(dir, file, &st.rules); err != nil {
		log.Printf("%s is unreadable (%v): no triggers until it is fixed", file, err)
		st.rules = nil
	}
	return st
}

// List is the rules, never nil.
func (st *Store) List() []Rule {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append(make([]Rule, 0, len(st.rules)), st.rules...)
}

// Replace keeps the list as given, each rule checked; an id missing or sent twice is made here.
func (st *Store) Replace(list []Rule) ([]Rule, error) {
	if len(list) > MaxRules {
		return nil, fmt.Errorf("%d triggers at most", MaxRules)
	}
	out := make([]Rule, 0, len(list))
	seen := map[string]bool{}
	for _, r := range list {
		if err := Check(&r); err != nil {
			return nil, err
		}
		for r.ID == "" || seen[r.ID] {
			r.ID = state.NewID()
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := state.SaveJSON(st.dir, file, out); err != nil {
		return nil, err
	}
	st.rules = out
	return append([]Rule(nil), out...), nil
}

// Covers says whether a rule looks at a pane of the machine whose workspace stands in dir, or whose shell is in cwd.
func (r Rule) Covers(machine, dir, cwd string) bool {
	if !r.Enabled || r.Machine != "" && r.Machine != machine {
		return false
	}
	return r.Dir == "" || within(dir, r.Dir) || within(cwd, r.Dir)
}

func within(p, dir string) bool {
	return p != "" && (p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/"))
}
