// Package repos is what remuda keeps and reads per repository of a machine, after Orca's project settings: where new
// worktrees start from and go, the branch an issue's worktree gets, the agent and the first prompts a new worktree starts
// with, the setup script it runs, and the named commands the run button starts (the Run actions). The settings live on
// the cockpit, keyed by machine and the repository's main folder; the reads (source.go, github.go) run on the machine.
package repos

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/state"
)

const file = "repos.json"

// Default words of a new worktree, the project's own settings over them (Orca's new-worktree defaults).
const (
	DefaultIssueBranch = "dev/feat/{number}"
	DefaultIssuePrompt = `Work on GitHub issue #{number} in {repo}: “{title}”. Read it first with: gh issue view {number} -R {repo} --comments`
	DefaultPRPrompt    = `Pick up GitHub pull request #{number} in {repo}: “{title}”. Read it first with: gh pr view {number} -R {repo} --comments`
)

// The limits of what a project's settings hold.
const (
	MaxActions = 20
	MaxCommand = 2000
	MaxScript  = 16 << 10
	MaxPrompt  = 4000
)

// Places a run action opens in; Icons the icons it may wear (ui/icons.tsx names).
var (
	Places = []string{"tab", "right", "down"}
	Icons  = []string{"play", "terminal", "circle-check", "sparkles", "layers", "settings", "zap", "refresh-cw", "monitor", "server", "code", "download"}
)

// Action is a named command the run button starts in a shell: in a new tab of the workspace, or split right or down
// beside the pane on screen, in a folder of the checkout.
type Action struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Folder  string `json:"folder,omitempty"` // relative to the workspace's checkout, "" for its top
	Icon    string `json:"icon,omitempty"`
	Place   string `json:"place,omitempty"` // tab | right | down, "" for tab
}

// Settings are one repository's: blank fields keep the defaults.
type Settings struct {
	Base        string   `json:"base,omitempty"`         // where a new branch starts, "" for origin's default branch
	Location    string   `json:"location,omitempty"`     // the folder new worktrees go in, "" for herdr's ~/.herdr/worktrees/<repository>
	IssueBranch string   `json:"issue_branch,omitempty"` // a worktree started from an issue: {number} and {slug} filled in
	Agent       string   `json:"agent,omitempty"`        // started in each new worktree: "" the last used, "shell" none
	IssuePrompt string   `json:"issue_prompt,omitempty"` // the agent's first prompt for an issue: {number} {title} {repo} {branch}
	PRPrompt    string   `json:"pr_prompt,omitempty"`    // and for a pull request
	Setup       string   `json:"setup,omitempty"`        // this cockpit's own setup script, after the repository's herdr.yaml one
	SetupWhen   string   `json:"setup_when,omitempty"`   // ask | run | skip, "" for run
	SetupWait   bool     `json:"setup_wait,omitempty"`   // the agent starts once setup is done, in the same terminal
	Actions     []Action `json:"actions,omitempty"`
}

// Store keeps every repository's settings in repos.json by machine and main folder.
type Store struct {
	dir string
	mu  sync.Mutex
	all map[string]Settings
}

func Open(dir string) *Store {
	st := &Store{dir: dir}
	if err := state.LoadJSON(dir, file, &st.all); err != nil {
		log.Printf("%s is unreadable (%v): no project settings until it is fixed", file, err)
	}
	if st.all == nil {
		st.all = map[string]Settings{}
	}
	return st
}

// key is the repository's place: its machine and main folder, in the machine's own spelling.
func key(machine, root string) string {
	return machine + "|" + rpath.For(root).Clean(root)
}

// Get is the repository's settings, the zero value for one never set.
func (st *Store) Get(machine, root string) Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return clone(st.all[key(machine, root)])
}

// Action is one of the repository's run actions by id.
func (st *Store) Action(machine, root, id string) (Action, bool) {
	for _, a := range st.Get(machine, root).Actions {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}

// Put keeps the repository's settings whole, checked and trimmed; actions without an id get one.
func (st *Store) Put(machine, root string, s Settings) (Settings, error) {
	s, err := Check(root, s)
	if err != nil {
		return Settings{}, err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	next := make(map[string]Settings, len(st.all)+1)
	for k, v := range st.all {
		next[k] = v
	}
	if empty(s) {
		delete(next, key(machine, root))
	} else {
		next[key(machine, root)] = s
	}
	if err := state.SaveJSON(st.dir, file, next); err != nil {
		return Settings{}, err
	}
	st.all = next
	return clone(s), nil
}

// Forget drops a removed machine's repositories.
func (st *Store) Forget(machine string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	next := map[string]Settings{}
	for k, v := range st.all {
		if !strings.HasPrefix(k, machine+"|") {
			next[k] = v
		}
	}
	if len(next) == len(st.all) {
		return
	}
	if err := state.SaveJSON(st.dir, file, next); err != nil {
		log.Printf("%s: %v", file, err)
		return
	}
	st.all = next
}

func clone(s Settings) Settings {
	s.Actions = append([]Action{}, s.Actions...)
	return s
}

func empty(s Settings) bool {
	return s.Base == "" && s.Location == "" && s.IssueBranch == "" && s.Agent == "" && s.IssuePrompt == "" && s.PRPrompt == "" &&
		s.Setup == "" && s.SetupWhen == "" && !s.SetupWait && len(s.Actions) == 0
}

// Check trims the settings and refuses what a worktree or a run could not take: a base or a branch pattern git would
// refuse, a location that is no absolute folder, an agent herdr does not know, a script or prompt past its size, an
// action without a name or a command, one that spans lines, climbs out of its checkout or opens anywhere unknown.
func Check(root string, s Settings) (Settings, error) {
	s.Base, s.Location, s.IssueBranch = strings.TrimSpace(s.Base), strings.TrimSpace(s.Location), strings.TrimSpace(s.IssueBranch)
	s.Agent, s.IssuePrompt, s.PRPrompt = strings.TrimSpace(s.Agent), strings.TrimSpace(s.IssuePrompt), strings.TrimSpace(s.PRPrompt)
	s.Setup, s.SetupWhen = strings.TrimSpace(s.Setup), strings.TrimSpace(s.SetupWhen)
	if s.Base != "" && !sessions.RefOK(s.Base) {
		return s, fmt.Errorf("%q is not a branch, tag or commit git would take", s.Base)
	}
	if s.Location != "" {
		style := rpath.For(root)
		if !style.IsAbs(s.Location) {
			return s, errors.New("the worktree location must be a whole folder path, like /Users/you/worktrees")
		}
		s.Location = style.Clean(s.Location)
	}
	if s.IssueBranch != "" && !sessions.RefOK(strings.NewReplacer("{number}", "1", "{slug}", "x").Replace(s.IssueBranch)) {
		return s, fmt.Errorf("%q would not make a branch name git takes", s.IssueBranch)
	}
	if s.Agent != "" && s.Agent != "shell" && !sessions.KnownAgent(s.Agent) {
		return s, fmt.Errorf("%q is not an agent herdr knows", s.Agent)
	}
	if utf8.RuneCountInString(s.IssuePrompt) > MaxPrompt || utf8.RuneCountInString(s.PRPrompt) > MaxPrompt {
		return s, fmt.Errorf("a first prompt holds at most %d characters", MaxPrompt)
	}
	if len(s.Setup) > MaxScript || strings.ContainsRune(s.Setup, 0) {
		return s, fmt.Errorf("the setup script holds at most %d KB of text", MaxScript>>10)
	}
	switch s.SetupWhen {
	case "", "ask", "run", "skip":
	default:
		return s, fmt.Errorf("when to run the setup script is ask, run or skip, not %q", s.SetupWhen)
	}
	if s.SetupWhen == "run" {
		s.SetupWhen = ""
	}
	if len(s.Actions) > MaxActions {
		return s, fmt.Errorf("at most %d run actions", MaxActions)
	}
	seen := map[string]bool{}
	for i := range s.Actions {
		a, err := checkAction(s.Actions[i])
		if err != nil {
			return s, err
		}
		for a.ID == "" || seen[a.ID] {
			a.ID = state.NewID()
		}
		seen[a.ID] = true
		s.Actions[i] = a
	}
	return s, nil
}

func checkAction(a Action) (Action, error) {
	a.ID, a.Name, a.Command = strings.TrimSpace(a.ID), strings.TrimSpace(a.Name), strings.TrimSpace(a.Command)
	a.Folder, a.Icon, a.Place = strings.Trim(strings.TrimSpace(a.Folder), "/"), strings.TrimSpace(a.Icon), strings.TrimSpace(a.Place)
	if a.Folder == "." {
		a.Folder = ""
	}
	switch {
	case a.Name == "":
		return a, errors.New("a run action needs a name")
	case utf8.RuneCountInString(a.Name) > 40:
		return a, fmt.Errorf("the run action %q has a name of more than 40 characters", a.Name)
	case a.Command == "":
		return a, fmt.Errorf("the run action %q needs a command", a.Name)
	case len(a.Command) > MaxCommand:
		return a, fmt.Errorf("the run action %q has a command of more than %d characters", a.Name, MaxCommand)
	case strings.ContainsAny(a.Command, "\r\n\x00"):
		return a, fmt.Errorf("the run action %q spans lines: join them with && or put them in a script", a.Name)
	case climbsOut(a.Folder):
		return a, fmt.Errorf("the run action %q names a folder outside its checkout: give one inside it, like web", a.Name)
	}
	if a.Icon == "" {
		a.Icon = "play"
	}
	if !oneOf(a.Icon, Icons) {
		return a, fmt.Errorf("the run action %q wears an icon remuda has not got: %q", a.Name, a.Icon)
	}
	if a.Place == "" {
		a.Place = "tab"
	}
	if !oneOf(a.Place, Places) {
		return a, fmt.Errorf("the run action %q opens in a tab, right or down, not %q", a.Name, a.Place)
	}
	return a, nil
}

// climbsOut says whether a folder named relative to a checkout leaves it: a "..", a home or drive, a backslash's own.
func climbsOut(folder string) bool {
	if strings.ContainsAny(folder, "\x00\\:") || strings.HasPrefix(folder, "~") {
		return true
	}
	for _, part := range strings.Split(folder, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func oneOf(v string, list []string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Fill puts a template's {number} {title} {repo} {branch} {slug} in place.
func Fill(template string, number int, title, repo, branch string) string {
	return strings.NewReplacer("{number}", fmt.Sprint(number), "{title}", title, "{repo}", repo, "{branch}", branch, "{slug}", TitleSlug(title)).Replace(template)
}

// TitleSlug is an issue's title as a branch's word: lower case, a dash for every other run of characters, 40 at most.
func TitleSlug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	out := b.String()
	if len(out) > 40 {
		out = strings.TrimRight(out[:40], "-")
	}
	return out
}
