package transcript

import "time"

// What an assistant turn spent: the time from the person's prompt to its last record, and the tokens of
// its API responses. One response is written as several records sharing message.id with the same usage,
// so a turn keeps one record per id, the last written, as usage.Accumulator does.

// spend notes one assistant record of the turn: its timestamp, and its usage when it has any.
func (t *Turn) spend(line []byte, ts string) {
	if ts != "" {
		t.ended = ts
	}
	r, ok := UsageOf(line)
	if !ok {
		return
	}
	for i := range t.Responses {
		if r.MessageID != "" && t.Responses[i].MessageID == r.MessageID {
			t.Responses[i] = r
			return
		}
	}
	t.Responses = append(t.Responses, r)
}

// settleSpending fills Took and Tokens of every assistant turn once the file is read.
func settleSpending(turns []Turn) {
	for i := range turns {
		t := &turns[i]
		if t.Role != "assistant" {
			continue
		}
		for _, r := range t.Responses {
			t.Tokens += r.Input + r.Output + r.CacheRead + r.CacheWrite5m + r.CacheWrite1h
		}
		if i == 0 || turns[i-1].Role != "user" {
			continue
		}
		asked, err := time.Parse(time.RFC3339Nano, turns[i-1].Ts)
		if err != nil {
			continue
		}
		ended, err := time.Parse(time.RFC3339Nano, t.ended)
		if err != nil {
			continue
		}
		if took := ended.Sub(asked).Seconds(); took > 0 {
			t.Took = took
		}
	}
}
