package usage

import (
	"math"
	"testing"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestPriceTurns(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "user"},
		{Role: "assistant", Responses: []transcript.UsageRecord{
			{MessageID: "a", Model: "claude-fable-5-1", Input: 1000, Output: 100},
			{MessageID: "b", Model: "claude-fable-5-1", CacheRead: 1_000_000},
		}},
		{Role: "assistant", Responses: []transcript.UsageRecord{{MessageID: "c", Model: "gpt-9", Output: 5}}},
		{Role: "assistant"},
	}
	priced := PriceTurns(turns)
	if turns[1].Cost != 0 {
		t.Fatal("the caller's slice was priced in place")
	}
	want := 1000/1e6*10 + 100/1e6*50 + 0.25
	if math.Abs(priced[1].Cost-want) > 1e-9 || priced[1].Model != "Fable 5.1" {
		t.Fatalf("priced %+v want %f", priced[1], want)
	}
	if priced[2].Cost != 0 || priced[2].Model != "gpt-9" {
		t.Fatalf("unknown model %+v", priced[2])
	}
	if priced[3].Cost != 0 || priced[3].Model != "" {
		t.Fatalf("no responses %+v", priced[3])
	}
}
