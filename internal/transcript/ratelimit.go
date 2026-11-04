package transcript

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// RateLimited reads the tail of a Claude Code session file: true when the conversation's last record is the harness's
// note that a subscription limit stopped the agent — an assistant record with isApiErrorMessage and error
// "rate_limit" — with when it was written and its words. A tail may start mid-line; that line is skipped.
func RateLimited(tail []byte) (time.Time, string, bool) {
	var at time.Time
	var text string
	hit := false
	for _, line := range bytes.Split(tail, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			IsMeta      bool   `json:"isMeta"`
			IsAPIError  bool   `json:"isApiErrorMessage"`
			Error       string `json:"error"`
			Timestamp   string `json:"timestamp"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.IsSidechain || rec.IsMeta || (rec.Type != "user" && rec.Type != "assistant") {
			continue
		}
		hit = rec.Type == "assistant" && rec.IsAPIError && rec.Error == "rate_limit"
		if hit {
			at, _ = time.Parse(time.RFC3339Nano, rec.Timestamp)
			text = errorText(rec.Message.Content)
		}
	}
	return at, text, hit
}

// errorText is an API error record's words: its content as a string, or its text blocks joined
func errorText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, " ")
}
