package usage

import (
	"math"
	"testing"
)

func TestTheCatalogKnowsClaudeCodes2_1_322Models(t *testing.T) {
	for _, c := range []struct {
		id, name          string
		input, output, cr float64
		window            int
	}{
		{"claude-fable-5-2", "Fable 5.2", 8, 40, 0.2, 1000000},
		{"claude-mythos-5-2", "Mythos 5.2", 8, 40, 0.2, 1000000},
		{"claude-haiku-5-5", "Haiku 5.5", 0.1, 0.5, 0.01, 1000000},
		{"claude-sonnet-5-5", "Sonnet 5.5", 2, 10, 0.1, 1000000},
		{"claude-fable-5-1", "Fable 5.1", 10, 50, 0.25, 1000000},
		{"claude-fable-5", "Fable 5", 10, 50, 1, 1000000},
	} {
		m, ok := Lookup(c.id)
		if !ok || m.Name != c.name || m.Rates.Input != c.input || m.Rates.Output != c.output || m.Rates.CacheRead != c.cr || m.Window != c.window {
			t.Errorf("%s: %+v, %v", c.id, m, ok)
		}
	}
	if m, _ := Lookup("us.anthropic.claude-fable-5-2"); m.Name != "Fable 5.2" {
		t.Errorf("a provider's spelling of Fable 5.2 read as %q, not the older Fable 5 its prefix also matches", m.Name)
	}
}

// Claude Code 2.1.322 prices fast mode from its own table for the models it names, and any other model at its usual
// rates whatever the record's speed says.
func TestFastModeIsPricedFromClaudeCodesTable(t *testing.T) {
	million := Tokens{Output: 1_000_000}
	for id, want := range map[string]float64{
		"claude-fable-5-2":  80,
		"claude-mythos-5-2": 80,
		"claude-opus-5-5":   40,
		"claude-opus-4-8":   50,
		"claude-opus-5":     50,
		"claude-opus-4-6":   150,
		"claude-opus-4-7":   150,
		"claude-sonnet-5-5": 10,
		"claude-fable-5-1":  50,
		"claude-haiku-4-5":  5,
	} {
		m, _ := Lookup(id)
		if got := Cost(m, million, "", "fast"); math.Abs(got-want) > 1e-9 {
			t.Errorf("a million output tokens of %s in fast mode: $%v, want $%v", id, got, want)
		}
	}
	haiku, _ := Lookup("claude-haiku-5-5")
	if got := Cost(haiku, Tokens{Input: 200_000}, "", "fast"); math.Abs(got-0.1) > 1e-12 {
		t.Errorf("a model with no fast rates keeps its long-prompt rule in fast mode: %v", got)
	}
}

func TestALongPromptIsPricedWholeAtTheLongRates(t *testing.T) {
	m, _ := Lookup("claude-haiku-5-5")
	short := Tokens{Input: 1000, CacheRead: 99_000, Output: 1_000_000}
	if got := Cost(m, short, "", ""); math.Abs(got-(1000*0.1+99_000*0.01+1_000_000*0.5)/1e6) > 1e-12 {
		t.Errorf("a prompt of 100k tokens, the limit itself, is a short one: %v", got)
	}
	long := Tokens{Input: 1001, CacheRead: 99_000, Output: 1_000_000}
	if got := Cost(m, long, "", ""); math.Abs(got-(1001*0.5+99_000*0.05+1_000_000*2.5)/1e6) > 1e-12 {
		t.Errorf("a prompt past 100k tokens is priced whole at the long rates: %v", got)
	}
	if opus, _ := Lookup("claude-opus-5-5"); opus.Long != nil {
		t.Error("only a tier with long rates has them")
	}
}
