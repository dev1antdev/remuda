package retire

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/tasks"
)

var fast48 = Table[0]

func TestTheTable(t *testing.T) {
	if fast48.Model != "claude-opus-4-8" || fast48.Mode != "fast" || fast48.Replacement != "claude-opus-5-5" || fast48.Source == "" {
		t.Fatalf("the Opus 4.8 fast mode entry: %+v", fast48)
	}
	if !fast48.Ends().Equal(time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("it ends %v", fast48.Ends())
	}
	if fast48.Failing(time.Date(2026, 11, 29, 23, 59, 0, 0, time.UTC)) || !fast48.Failing(time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("it fails from the start of its day")
	}
	if (Retirement{On: "someday"}).Failing(time.Now()) {
		t.Fatal("a day that does not parse never fails")
	}
}

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-4-8":                            "claude-opus-4-8",
		" Claude-Opus-4-8[1m] ":                      "claude-opus-4-8",
		"claude-opus-4-8-20251101":                   "claude-opus-4-8",
		"claude-opus-4-8@20251101":                   "claude-opus-4-8",
		"us.anthropic.claude-opus-4-8-20251101-v1:0": "claude-opus-4-8",
		"anthropic/claude-opus-4-8":                  "claude-opus-4-8",
		"opus":                                       "opus",
		"claude-opus-4":                              "claude-opus-4",
	} {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplies(t *testing.T) {
	cases := []struct {
		agent, model string
		fast, want   bool
	}{
		{"claude", "claude-opus-4-8", true, true},
		{"", "claude-opus-4-8[1m]", true, true},
		{"claude", "claude-opus-4-8", false, false}, // standard Opus 4.8 stays
		{"claude", "claude-opus-5-5", true, false},
		{"claude", "opus", true, false}, // the alias names the newest Opus
		{"claude", "claude-opus-4", true, false},
		{"codex", "claude-opus-4-8", true, false},
	}
	for _, c := range cases {
		if got := fast48.Applies(c.agent, c.model, c.fast); got != c.want {
			t.Errorf("Applies(%q, %q, %v) = %v", c.agent, c.model, c.fast, got)
		}
	}
	whole := Retirement{Agent: "claude", Model: "claude-sonnet-4-0"}
	if !whole.Applies("claude", "claude-sonnet-4-0-20250514", false) {
		t.Fatal("a whole model's retirement takes every mode")
	}
}

func TestWhatTheArgumentsSay(t *testing.T) {
	if fast, said := FastIn(`--settings '{"fastMode": true}'`); !fast || !said {
		t.Fatal("fast mode through --settings")
	}
	if fast, said := FastIn(`--settings '{"fastMode":true}' --settings '{"fastMode" : false}'`); fast || !said {
		t.Fatal("the last word wins")
	}
	if _, said := FastIn("--permission-mode plan"); said {
		t.Fatal("arguments that say nothing of it")
	}
	if fast, said := FastIn(`--settings "{\"fastMode\": true}"`); !fast || !said {
		t.Fatal("fast mode through --settings in double quotes, its own quotes escaped")
	}
	if ModelIn(`--model \"claude-opus-4-8\"`) != "claude-opus-4-8" {
		t.Fatal("a --model in escaped quotes")
	}
	if ModelIn("--model claude-opus-4-8 --verbose") != "claude-opus-4-8" || ModelIn(`--model="claude-opus-4-8[1m]"`) != "claude-opus-4-8[1m]" || ModelIn("--verbose") != "" {
		t.Fatal("a --model in the arguments")
	}
}

func TestFind(t *testing.T) {
	before := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	defaults := map[string]Defaults{
		"gog": {Model: "claude-opus-4-8", Fast: true},
		"pi":  {},
	}
	settings := []Setting{
		{Kind: "recipe", ID: "r1", Agent: "claude", Model: "claude-opus-4-8", Args: `--settings '{"fastMode":true}'`}, // any machine: fails on both
		{Kind: "recipe", ID: "r2", Agent: "claude"},                                            // the default: Opus 4.8 fast on gog alone
		{Kind: "schedule", ID: "s1", Machine: "pi", Agent: "claude", Model: "claude-opus-4-8"}, // standard speed there
		{Kind: "schedule", ID: "s2", Machine: "gog", Agent: "claude", Model: "claude-opus-4-8", Args: `--settings '{"fastMode":false}'`},
		{Kind: "project", ID: "7", Machine: "gog", Model: "claude-opus-5-5"},
		{Kind: "stage", ID: "7/1", Machine: "gog"},
		{Kind: "recipe", ID: "r3", Agent: "codex", Model: "claude-opus-4-8", Args: `{"fastMode":true}`},
	}
	panes := []Pane{
		{Machine: "pi", Pane: "w1:p1", Agent: "claude", Model: "claude-opus-4-8", Speed: "fast"},
		{Machine: "pi", Pane: "w1:p2", Agent: "claude", Model: "claude-opus-4-8"},
	}
	hits := Find(Table, settings, panes, defaults, before)
	got := map[string]bool{}
	for _, h := range hits {
		got[h.Kind+":"+h.ID+"@"+h.Machine] = true
		if h.Failing || h.Model != "claude-opus-4-8" {
			t.Errorf("a hit before the day: %+v", h)
		}
	}
	want := []string{"recipe:r1@gog", "recipe:r1@pi", "recipe:r2@gog", "stage:7/1@gog", "pane:w1:p1@pi"}
	if len(hits) != len(want) {
		t.Fatalf("hits %v, want %v", got, want)
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	after := Find(Table, settings[:1], nil, map[string]Defaults{"gog": {}}, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	if len(after) != 1 || !after[0].Failing {
		t.Fatalf("past the day it fails: %+v", after)
	}
	if none := Find(Table, settings[:1], nil, nil, before); len(none) != 0 {
		t.Fatal("no machine with Claude Code, nowhere it would start")
	}
}

func TestServiceKeepsTheHitsAWhile(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	reads := 0
	s := &Service{
		Settings: func() []Setting {
			reads++
			return []Setting{{Kind: "schedule", ID: "s1", Machine: "gog", Model: "claude-opus-4-8"}}
		},
		Defaults: func() map[string]Defaults { return map[string]Defaults{"gog": {Fast: true}} },
		now:      func() time.Time { return now },
	}
	if hits := s.On("gog"); len(hits) != 1 || hits[0].Kind != "schedule" {
		t.Fatalf("gog: %+v", hits)
	}
	if len(s.On("pi")) != 0 || reads != 1 {
		t.Fatalf("another machine, from the same look: %d reads", reads)
	}
	now = now.Add(Every)
	s.On("gog")
	if reads != 2 {
		t.Fatal("not looked for again once Every passed")
	}
	s.Moved()
	s.On("gog")
	if reads != 3 {
		t.Fatal("a setting changed and the hits were not looked for again")
	}
}

func TestSources(t *testing.T) {
	r := Recipes([]recipes.Recipe{{ID: "a", Agent: "claude", Model: "m", Args: "x", Machine: "gog"}, {ID: "b", Agent: "shell"}})
	if len(r) != 1 || r[0] != (Setting{Kind: "recipe", ID: "a", Machine: "gog", Agent: "claude", Model: "m", Args: "x"}) {
		t.Fatalf("recipes: %+v", r)
	}
	s := Schedules([]schedule.Schedule{
		{ID: "on", Kind: schedule.KindAgent, Enabled: true, Machine: "gog", Agent: "claude", Model: "m"},
		{ID: "off", Kind: schedule.KindAgent, Machine: "gog", Agent: "claude", Model: "m"},
		{ID: "prompt", Kind: "prompt", Enabled: true, Machine: "gog"},
	})
	if len(s) != 1 || s[0].ID != "on" {
		t.Fatalf("an agent schedule switched on alone: %+v", s)
	}
	p := Projects([]tasks.Project{
		{ID: 3, Automation: tasks.Automation{Machine: "gog", Agent: "claude", Model: "m", Args: "x", Stages: []tasks.Stage{{Agent: "codex", Model: "gpt"}, {Model: "n"}}}},
		{ID: 4, Automation: tasks.Automation{Max: 1, Stages: []tasks.Stage{}}},
		{ID: 5, Archived: true, Automation: tasks.Automation{Machine: "gog", Model: "m"}},
	})
	if len(p) != 3 || p[0] != (Setting{Kind: "project", ID: "3", Machine: "gog", Agent: "claude", Model: "m", Args: "x"}) || p[2] != (Setting{Kind: "stage", ID: "3/2", Machine: "gog", Model: "n"}) {
		t.Fatalf("projects: %+v", p)
	}
}
