package recap

import (
	"sort"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

// Span is one stretch of an agent's day: working from a prompt to the end of its answer, waiting from a question
// it asked until the person answered, idle in between; Turn is the first record of the turn it belongs to, the jump
// the page makes into the chat.
type Span struct {
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	State string    `json:"state"` // working | waiting | idle
	Turn  string    `json:"turn,omitempty"`
}

// IdleGap is the least gap between two stretches of work that the strip draws as idle.
const IdleGap = 2 * time.Minute

// Spans lays the turns out in time, clipped to [since, now]: a zero since keeps everything, a zero now never clips
// the end. A question still open waits until now.
func Spans(turns []transcript.Turn, since, now time.Time) []Span {
	var working, waiting []Span
	for i := range turns {
		t := &turns[i]
		if t.Role != "assistant" {
			continue
		}
		from, to, turn, ok := turnSpan(turns, i)
		if !ok {
			continue
		}
		working = append(working, Span{From: from, To: to, State: "working", Turn: turn})
		for _, p := range t.Parts {
			if p.Type != "tool" || p.ToolName != "AskUserQuestion" || p.IsError {
				continue
			}
			asked, err := time.Parse(time.RFC3339Nano, p.At)
			if err != nil {
				continue
			}
			answered := asked.Add(time.Duration(p.Took * float64(time.Second)))
			if p.Took == 0 {
				answered = nextPrompt(turns, i, now)
			}
			if answered.After(asked) {
				waiting = append(waiting, Span{From: asked, To: answered, State: "waiting", Turn: turn})
			}
		}
	}
	spans := append(cut(working, waiting), waiting...)
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].From.Before(spans[j].From) })
	spans = clip(spans, since, now)
	return withIdle(spans, since, now)
}

// turnSpan is when an assistant turn's work ran: from the prompt before it for as long as it took, else from its
// own first record to its last tool result.
func turnSpan(turns []transcript.Turn, i int) (from, to time.Time, turn string, ok bool) {
	t := &turns[i]
	if i > 0 && turns[i-1].Role == "user" && t.Took > 0 {
		asked, err := time.Parse(time.RFC3339Nano, turns[i-1].Ts)
		if err == nil {
			return asked, asked.Add(time.Duration(t.Took * float64(time.Second))), firstID(&turns[i-1], t), true
		}
	}
	started, err := time.Parse(time.RFC3339Nano, t.Ts)
	if err != nil {
		return from, to, "", false
	}
	ended := started
	for _, p := range t.Parts {
		if p.Type != "tool" {
			continue
		}
		if at, err := time.Parse(time.RFC3339Nano, p.At); err == nil {
			if end := at.Add(time.Duration(p.Took * float64(time.Second))); end.After(ended) {
				ended = end
			}
		}
	}
	if !ended.After(started) {
		return from, to, "", false
	}
	return started, ended, firstID(t), true
}

// firstID is the first record uuid among the turns given, the prompt's before the answer's.
func firstID(turns ...*transcript.Turn) string {
	for _, t := range turns {
		if len(t.IDs) > 0 {
			return t.IDs[0]
		}
	}
	return ""
}

// nextPrompt is when the person next wrote after turn i, else now.
func nextPrompt(turns []transcript.Turn, i int, now time.Time) time.Time {
	for j := i + 1; j < len(turns); j++ {
		if turns[j].Role != "user" {
			continue
		}
		if at, err := time.Parse(time.RFC3339Nano, turns[j].Ts); err == nil {
			return at
		}
	}
	return now
}

// cut takes the waiting windows out of the working spans: a question asked mid-turn splits its turn's work in two.
func cut(working, waiting []Span) []Span {
	out := working
	for _, w := range waiting {
		var next []Span
		for _, s := range out {
			if !w.From.Before(s.To) || !w.To.After(s.From) {
				next = append(next, s)
				continue
			}
			if w.From.After(s.From) {
				next = append(next, Span{From: s.From, To: w.From, State: s.State, Turn: s.Turn})
			}
			if w.To.Before(s.To) {
				next = append(next, Span{From: w.To, To: s.To, State: s.State, Turn: s.Turn})
			}
		}
		out = next
	}
	return out
}

// clip keeps what falls in [since, now], trimming a span that straddles either edge.
func clip(spans []Span, since, now time.Time) []Span {
	out := make([]Span, 0, len(spans))
	for _, s := range spans {
		if !since.IsZero() {
			if !s.To.After(since) {
				continue
			}
			if s.From.Before(since) {
				s.From = since
			}
		}
		if !now.IsZero() {
			if !s.From.Before(now) {
				continue
			}
			if s.To.After(now) {
				s.To = now
			}
		}
		out = append(out, s)
	}
	return out
}

// withIdle fills every gap of IdleGap or more between the spans, and before the first and after the last within
// [since, now], with an idle span.
func withIdle(spans []Span, since, now time.Time) []Span {
	if len(spans) == 0 {
		if !since.IsZero() && !now.IsZero() && now.Sub(since) >= IdleGap {
			return []Span{{From: since, To: now, State: "idle"}}
		}
		return []Span{}
	}
	out := make([]Span, 0, 2*len(spans)+1)
	last := since
	for _, s := range spans {
		if !last.IsZero() && s.From.Sub(last) >= IdleGap {
			out = append(out, Span{From: last, To: s.From, State: "idle"})
		}
		out = append(out, s)
		if s.To.After(last) {
			last = s.To
		}
	}
	if !now.IsZero() && now.Sub(last) >= IdleGap {
		out = append(out, Span{From: last, To: now, State: "idle"})
	}
	return out
}
