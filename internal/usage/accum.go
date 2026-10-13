package usage

import (
	"github.com/dev1antdev/remuda/internal/transcript"
)

// Totals is a sum with its price.
type Totals struct {
	Tokens
	Cost    float64 `json:"cost"`
	Turns   int     `json:"turns"`   // API responses
	Unknown int     `json:"unknown"` // responses from a model with no price here (counted, not priced)
}

func (t *Totals) add(o Totals) {
	t.Tokens.Add(o.Tokens)
	t.Cost += o.Cost
	t.Turns += o.Turns
	t.Unknown += o.Unknown
}

// Accumulator sums records, counting each message id once: the records of one response sit together, the last wins.
// The response still being written is held until the next id, so the file can be fed in pieces.
type Accumulator struct {
	Totals
	ByModel map[string]*Totals `json:"by_model,omitempty"`
	Last    transcript.UsageRecord
	// OnCommit, when set, sees every response once, priced
	OnCommit func(r transcript.UsageRecord, t Totals)
	pendID   string
	pend     transcript.UsageRecord
	hasPend  bool
}

func tokensOf(r transcript.UsageRecord) Tokens {
	return Tokens{Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, CacheWrite5m: r.CacheWrite5m, CacheWrite1h: r.CacheWrite1h, WebSearch: r.WebSearch}
}

// price is one response's totals.
func price(r transcript.UsageRecord) Totals {
	t := Totals{Tokens: tokensOf(r), Turns: 1}
	if m, ok := Lookup(r.Model); ok {
		t.Cost = Cost(m, t.Tokens, r.Geo, r.Speed)
	} else {
		t.Unknown = 1
	}
	return t
}

func (a *Accumulator) Add(r transcript.UsageRecord) {
	if a.hasPend && r.MessageID != "" && r.MessageID == a.pendID {
		a.pend = r // the same response, written again with a later block
		return
	}
	a.Flush()
	a.pendID, a.pend, a.hasPend = r.MessageID, r, true
}

// Flush commits the response being held; call it before reading totals.
func (a *Accumulator) Flush() {
	if !a.hasPend {
		return
	}
	r := a.pend
	a.hasPend = false
	t := price(r)
	a.Totals.add(t)
	if a.OnCommit != nil {
		a.OnCommit(r, t)
	}
	if a.ByModel == nil {
		a.ByModel = map[string]*Totals{}
	}
	key := r.Model
	if m, ok := Lookup(r.Model); ok {
		key = m.ID
	}
	bm := a.ByModel[key]
	if bm == nil {
		bm = &Totals{}
		a.ByModel[key] = bm
	}
	bm.add(t)
	if r.At.After(a.Last.At) || a.Last.MessageID == "" {
		a.Last = r
	}
}

// Sum is the totals and the latest response with the pending one counted, without committing it.
func (a *Accumulator) Sum() (Totals, transcript.UsageRecord) {
	t, last := a.Totals, a.Last
	if a.hasPend {
		t.add(price(a.pend))
		if a.pend.At.After(last.At) || last.MessageID == "" {
			last = a.pend
		}
	}
	return t, last
}
