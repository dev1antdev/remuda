package retire

import (
	"sort"
	"sync"
	"time"
)

// Setting is a model remuda keeps for an agent it starts: a recipe, an agent schedule, a project's agents, a stage.
type Setting struct {
	Kind    string // recipe | schedule | project | stage
	ID      string // the recipe's or the schedule's id, the project's, "<project>/<n>" for a stage
	Machine string // where it starts; "" for any machine, as a recipe for any folder
	Agent   string // "" reads as Claude Code
	Model   string // "" for the machine's default
	Args    string // the extra arguments as the person typed them
}

// Pane is an agent pane as its meter last read it: the model of its last response and the speed it ran at.
type Pane struct {
	Machine string
	Pane    string
	Agent   string
	Model   string
	Speed   string // "fast", or "" for the standard speed
}

// Defaults is what Claude Code's user settings on a machine choose for a session that names nothing: its model and
// whether fast mode is saved on; only a machine with Claude Code has any.
type Defaults struct {
	Model string
	Fast  bool
}

// Hit is a setting or a pane on a retirement's way, on a machine; no names, so a viewer link reads no recipe's or
// project's — the page names a pane by its workspace, and a form says its own. The retirement goes by its key, its
// agent, model and mode, which the roster's table answers.
type Hit struct {
	Agent   string `json:"agent"`
	Model   string `json:"model"`
	Mode    string `json:"mode,omitempty"`
	Kind    string `json:"kind"` // recipe | schedule | project | stage | pane
	ID      string `json:"id"`   // the setting's id, or the pane's
	Machine string `json:"machine"`
	Failing bool   `json:"failing"` // the day has come: its requests fail now
}

// Find is every setting and pane on a retirement's way at now; a setting for any machine is weighed on each machine
// with Claude Code, its default model and fast mode there, and is a hit on every one where it would fail.
func Find(table []Retirement, settings []Setting, panes []Pane, defaults map[string]Defaults, now time.Time) []Hit {
	var hits []Hit
	machines := make([]string, 0, len(defaults))
	for id := range defaults {
		machines = append(machines, id)
	}
	sort.Strings(machines)
	for _, s := range settings {
		where := []string{s.Machine}
		if s.Machine == "" {
			where = machines
		}
		for _, machine := range where {
			for _, r := range table {
				if r.Applies(s.Agent, settingModel(s, defaults[machine]), settingFast(s, defaults[machine])) {
					hits = append(hits, r.hit(s.Kind, s.ID, machine, now))
				}
			}
		}
	}
	for _, p := range panes {
		for _, r := range table {
			if r.Applies(p.Agent, p.Model, p.Speed == "fast") {
				hits = append(hits, r.hit("pane", p.Pane, p.Machine, now))
			}
		}
	}
	return hits
}

// hit is what of a kind is on the retirement's way on a machine, the retirement named by its key
func (r Retirement) hit(kind, id, machine string, now time.Time) Hit {
	return Hit{Agent: r.Agent, Model: r.Model, Mode: r.Mode, Kind: kind, ID: id, Machine: machine, Failing: r.Failing(now)}
}

// settingModel is the model a setting starts on there: a --model of its arguments, its own, else the machine's default
func settingModel(s Setting, d Defaults) string {
	if m := ModelIn(s.Args); m != "" {
		return m
	}
	if s.Model != "" {
		return s.Model
	}
	return d.Model
}

// settingFast is whether a setting starts in fast mode there: what its arguments say, else the machine's saved choice
func settingFast(s Setting, d Defaults) bool {
	if fast, said := FastIn(s.Args); said {
		return fast
	}
	return d.Fast
}

// Every is how long the hits stand before they are looked for again.
var Every = 30 * time.Second

// Service keeps the hits for the roster, looked for at most every Every.
type Service struct {
	Settings func() []Setting
	Panes    func() []Pane
	Defaults func() map[string]Defaults // machine id → Claude Code's defaults there

	now  func() time.Time
	mu   sync.Mutex
	at   time.Time
	hits []Hit
}

// Moved says a recipe, a schedule or a project's agents changed: the next look finds the hits again.
func (s *Service) Moved() {
	s.mu.Lock()
	s.at = time.Time{}
	s.mu.Unlock()
}

// On is the hits on a machine.
func (s *Service) On(machine string) []Hit {
	var out []Hit
	for _, h := range s.all() {
		if h.Machine == machine {
			out = append(out, h)
		}
	}
	return out
}

func (s *Service) all() []Hit {
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.at.IsZero() && now.Sub(s.at) < Every {
		return s.hits
	}
	var settings []Setting
	var panes []Pane
	defaults := map[string]Defaults{}
	if s.Settings != nil {
		settings = s.Settings()
	}
	if s.Panes != nil {
		panes = s.Panes()
	}
	if s.Defaults != nil {
		defaults = s.Defaults()
	}
	s.hits, s.at = Find(Table, settings, panes, defaults, now), now
	return s.hits
}
