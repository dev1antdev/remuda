// Package usage counts what the agents spend: tokens per transcript record,
// priced the way Claude Code prices them (its baked model catalog, read
// from the installed 2.1.285: nine tiers, the 1h cache tier, the US-geo
// surcharge), and how full a session's context is against its model's
// window. Nothing here talks to Anthropic: it is the API-equivalent cost of
// what the transcripts say, which on a subscription is a measure, not a
// bill.
package usage

import (
	"regexp"
	"strings"
)

// Rates are USD per million tokens (web search: per request).
type Rates struct {
	Input, Output, CacheWrite5m, CacheWrite1h, CacheRead, WebSearch float64
}

type Model struct {
	ID     string
	Name   string
	Rates  Rates
	Window int // context window, tokens
}

var tiers = map[string]Rates{
	"tier_2_10":                  {2, 10, 2.5, 4, 0.2, 0.01},
	"tier_3_15":                  {3, 15, 3.75, 6, 0.3, 0.01},
	"tier_5_25":                  {5, 25, 6.25, 10, 0.5, 0.01},
	"tier_15_75":                 {15, 75, 18.75, 30, 1.5, 0.01},
	"tier_10_50":                 {10, 50, 12.5, 20, 1, 0.01},
	"tier_10_50_cache_read_0_25": {10, 50, 12.5, 20, 0.25, 0.01},
	"tier_4_20_cache_read_0_20":  {4, 20, 5, 8, 0.2, 0.01},
	"haiku_35":                   {0.8, 4, 1, 1.6, 0.08, 0.01},
	"haiku_45":                   {1, 5, 1.25, 2, 0.1, 0.01},
}

// the catalog, longest ids first so a prefix match takes the most specific
var models = []struct {
	id, name, tier string
	window         int
}{
	{"claude-3-5-haiku", "Haiku 3.5", "haiku_35", 200000},
	{"claude-haiku-4-5", "Haiku 4.5", "haiku_45", 200000},
	{"claude-3-5-sonnet", "Sonnet 3.5", "tier_3_15", 200000},
	{"claude-3-7-sonnet", "Sonnet 3.7", "tier_3_15", 200000},
	{"claude-sonnet-4-0", "Sonnet 4", "tier_3_15", 200000},
	{"claude-sonnet-4-5", "Sonnet 4.5", "tier_3_15", 200000},
	{"claude-sonnet-4-6", "Sonnet 4.6", "tier_3_15", 200000},
	{"claude-sonnet-5-5", "Sonnet 5.5", "tier_2_10", 1000000},
	{"claude-sonnet-5", "Sonnet 5", "tier_2_10", 1000000},
	{"claude-opus-4-0", "Opus 4", "tier_15_75", 200000},
	{"claude-opus-4-1", "Opus 4.1", "tier_15_75", 200000},
	{"claude-opus-4-5", "Opus 4.5", "tier_5_25", 200000},
	{"claude-opus-4-6", "Opus 4.6", "tier_5_25", 200000},
	{"claude-opus-4-7", "Opus 4.7", "tier_5_25", 1000000},
	{"claude-opus-4-8", "Opus 4.8", "tier_5_25", 1000000},
	{"claude-opus-5-5", "Opus 5.5", "tier_4_20_cache_read_0_20", 1000000},
	{"claude-opus-5", "Opus 5", "tier_5_25", 1000000},
	{"claude-fable-5-1", "Fable 5.1", "tier_10_50_cache_read_0_25", 1000000},
	{"claude-fable-5", "Fable 5", "tier_10_50", 1000000},
	{"claude-mythos-5-1", "Mythos 5.1", "tier_10_50_cache_read_0_25", 1000000},
	{"claude-mythos-5", "Mythos 5", "tier_10_50", 1000000},
	// older dated ids: "claude-sonnet-4-20250514", "claude-opus-4-20250514"
	{"claude-sonnet-4", "Sonnet 4", "tier_3_15", 200000},
	{"claude-opus-4", "Opus 4", "tier_15_75", 200000},
}

var dateSuffix = regexp.MustCompile(`-\d{8}$`)

// Lookup finds a model by its id as the transcripts spell it — the
// canonical id, a dated one ("claude-sonnet-4-5-20250929"), or a provider's
// prefix ("us.anthropic.…"). The second value is false for an unknown one.
func Lookup(model string) (Model, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" || strings.HasPrefix(m, "<") {
		return Model{}, false
	}
	if i := strings.Index(m, "claude-"); i > 0 {
		m = m[i:] // "us.anthropic.claude-…", "anthropic/claude-…"
	}
	m = dateSuffix.ReplaceAllString(m, "")
	for _, c := range models {
		if m == c.id || strings.HasPrefix(m, c.id+"-") || strings.HasPrefix(m, c.id+"@") {
			return Model{ID: c.id, Name: c.name, Rates: tiers[c.tier], Window: c.window}, true
		}
	}
	return Model{}, false
}

// FastRates are what Claude Code charges in fast mode (its z9e): Opus 5.5
// one table, Opus 4.8 and 5 another, everything else the dearest.
func FastRates(m Model) Rates {
	switch m.ID {
	case "claude-opus-5-5":
		return Rates{8, 40, 10, 16, 0.4, 0.01}
	case "claude-opus-4-8", "claude-opus-5":
		return Rates{10, 50, 12.5, 20, 1, 0.01}
	}
	return Rates{30, 150, 37.5, 60, 3, 0.01}
}

// Tokens are one record's or a sum's counts.
type Tokens struct {
	Input        int64 `json:"input"`
	Output       int64 `json:"output"`
	CacheRead    int64 `json:"cache_read"`
	CacheWrite5m int64 `json:"cache_write"`
	CacheWrite1h int64 `json:"cache_write_1h,omitempty"`
	WebSearch    int64 `json:"web_search,omitempty"`
}

func (t *Tokens) Add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite5m += o.CacheWrite5m
	t.CacheWrite1h += o.CacheWrite1h
	t.WebSearch += o.WebSearch
}

// Context is what the record's prompt held: everything the model read.
func (t Tokens) Context() int64 { return t.Input + t.CacheRead + t.CacheWrite5m + t.CacheWrite1h }

// Cost prices one record the way Claude Code does: the 1h cache tier where
// the API said so, fast mode from its own table, a tenth more when
// inference was pinned to the US (not on web search).
func Cost(m Model, t Tokens, geo, speed string) float64 {
	r := m.Rates
	if speed == "fast" {
		r = FastRates(m)
	}
	c := float64(t.Input)/1e6*r.Input + float64(t.Output)/1e6*r.Output + float64(t.CacheRead)/1e6*r.CacheRead +
		float64(t.CacheWrite5m)/1e6*r.CacheWrite5m + float64(t.CacheWrite1h)/1e6*r.CacheWrite1h
	if geo == "us" {
		c *= 1.1
	}
	return c + float64(t.WebSearch)*r.WebSearch
}
