package usage

import "github.com/dev1antdev/remuda/internal/transcript"

// PriceTurns puts a cost and a model name on every assistant turn that carries its API responses, in a
// copy of the slice; a response from a model the catalog does not know is counted in the tokens, not priced.
func PriceTurns(turns []transcript.Turn) []transcript.Turn {
	out := append([]transcript.Turn(nil), turns...)
	for i := range out {
		t := &out[i]
		if len(t.Responses) == 0 {
			continue
		}
		for _, r := range t.Responses {
			t.Cost += price(r).Cost
		}
		last := t.Responses[len(t.Responses)-1]
		t.Model = last.Model
		if m, ok := Lookup(last.Model); ok {
			t.Model = m.Name
		}
	}
	return out
}
