package usage

import (
	"math"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestLookupSpellings(t *testing.T) {
	cases := map[string]string{
		"claude-fable-5-1":                             "claude-fable-5-1",
		"claude-sonnet-4-5-20250929":                   "claude-sonnet-4-5",
		"claude-opus-4-1-20250805":                     "claude-opus-4-1",
		"claude-3-5-haiku-20241022":                    "claude-3-5-haiku",
		"us.anthropic.claude-sonnet-4-5-20250929-v1:0": "claude-sonnet-4-5",
		"claude-opus-5-5":                              "claude-opus-5-5",
		"claude-opus-5":                                "claude-opus-5",
		"claude-sonnet-4-20250514":                     "claude-sonnet-4",
	}
	for in, want := range cases {
		m, ok := Lookup(in)
		if !ok || m.ID != want {
			t.Errorf("%q → %q %v, want %q", in, m.ID, ok, want)
		}
	}
	for _, bad := range []string{"<synthetic>", "", "gpt-5", "claude-nope-9"} {
		if _, ok := Lookup(bad); ok {
			t.Errorf("%q looked up", bad)
		}
	}
	if m, _ := Lookup("claude-fable-5-1"); m.Window != 1000000 || m.Rates.CacheRead != 0.25 || m.Rates.Output != 50 {
		t.Fatalf("fable rates %+v", m)
	}
	if m, _ := Lookup("claude-opus-4-1"); m.Window != 200000 || m.Rates.Input != 15 {
		t.Fatalf("opus 4.1 %+v", m)
	}
}

func TestCostMatchesClaudeCode(t *testing.T) {
	// a real Fable 5.1 record: 32 in, 2276 written to the 1h cache, 588536 read, 888 out
	m, _ := Lookup("claude-fable-5-1")
	c := Cost(m, Tokens{Input: 32, CacheWrite1h: 2276, CacheRead: 588536, Output: 888}, "not_available", "standard")
	want := 32/1e6*10 + 888/1e6*50 + 588536/1e6*0.25 + 2276/1e6*20
	if math.Abs(c-want) > 1e-9 {
		t.Fatalf("cost %f want %f", c, want)
	}
	if us := Cost(m, Tokens{Output: 1000}, "us", ""); math.Abs(us-0.055) > 1e-9 {
		t.Fatalf("us surcharge %f", us)
	}
	if ws := Cost(m, Tokens{WebSearch: 3}, "us", ""); math.Abs(ws-0.03) > 1e-9 {
		t.Fatalf("web search is not surcharged: %f", ws)
	}
	opus, _ := Lookup("claude-opus-5-5")
	if fast := Cost(opus, Tokens{Output: 1000}, "", "fast"); math.Abs(fast-0.04) > 1e-9 {
		t.Fatalf("opus 5.5 fast %f", fast)
	}
	if fast := Cost(m, Tokens{Output: 1000}, "", "fast"); math.Abs(fast-0.15) > 1e-9 {
		t.Fatalf("fable fast (the dearest table) %f", fast)
	}
}

func TestUsageOfAndAccumulatorDedupes(t *testing.T) {
	line := `{"type":"assistant","timestamp":"2026-10-07T10:00:00.000Z","cwd":"/p","message":{"id":"msg_1","model":"claude-fable-5-1","usage":{"input_tokens":32,"output_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":500,"cache_creation":{"ephemeral_1h_input_tokens":300},"server_tool_use":{"web_search_requests":1},"inference_geo":"us"}}}`
	r, ok := transcript.UsageOf([]byte(line))
	if !ok || r.MessageID != "msg_1" || r.CacheWrite1h != 300 || r.CacheWrite5m != 200 || r.WebSearch != 1 || r.Geo != "us" || r.Cwd != "/p" {
		t.Fatalf("%+v %v", r, ok)
	}
	if _, ok := transcript.UsageOf([]byte(`{"type":"user","message":{"role":"user","content":"hi"}}`)); ok {
		t.Fatal("a user record has usage?")
	}
	apiErr := `{"type":"assistant","isApiErrorMessage":true,"timestamp":"2026-10-07T10:00:00.000Z","message":{"id":"e","model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0}}}`
	if _, ok := transcript.UsageOf([]byte(apiErr)); ok {
		t.Fatal("an API error counted as a response")
	}
	fastLine := `{"type":"assistant","timestamp":"2026-10-07T10:00:00.000Z","message":{"id":"f","model":"claude-opus-5-5","usage":{"input_tokens":0,"output_tokens":1000,"speed":"fast"}}}`
	if fr, ok := transcript.UsageOf([]byte(fastLine)); !ok || fr.Speed != "fast" {
		t.Fatalf("speed %+v", fr)
	}
	var a Accumulator
	a.Add(r)
	r2 := r
	r2.Output = 40 // the same response, a later block
	a.Add(r2)
	r3 := r
	r3.MessageID, r3.At = "msg_2", r.At.Add(time.Minute)
	a.Add(r3)
	a.Flush()
	if a.Turns != 2 || a.Output != 50 || a.Input != 64 {
		t.Fatalf("totals %+v", a.Totals)
	}
	if a.Last.MessageID != "msg_2" || a.ByModel["claude-fable-5-1"].Turns != 2 {
		t.Fatalf("last %+v by model %+v", a.Last, a.ByModel)
	}
	a.Add(transcript.UsageRecord{MessageID: "x", Model: "gpt-9", Output: 5})
	a.Flush()
	if a.Unknown != 1 || a.Turns != 3 {
		t.Fatalf("unknown %+v", a.Totals)
	}
	a.Add(transcript.UsageRecord{MessageID: "y", Model: "claude-fable-5-1", Output: 100, At: r.At.Add(time.Hour)})
	sum, last := a.Sum()
	if sum.Turns != 4 || sum.Output != 155 || last.MessageID != "y" || a.Turns != 3 || a.ByModel["claude-fable-5-1"].Turns != 2 {
		t.Fatalf("sum %+v last %s committed %d", sum, last.MessageID, a.Turns)
	}
}
