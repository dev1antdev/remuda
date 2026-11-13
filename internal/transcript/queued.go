package transcript

import (
	"encoding/json"
	"regexp"
	"strings"
)

// A message the person typed while the agent worked: Claude Code queues it and hands it to the model mid-turn, written
// as an attachment record with no user record of its own —
// {"type":"attachment","attachment":{"type":"queued_command","origin":{"kind":"human"},"commandMode":"prompt","prompt":"…"}}.
// Another agent's message reaches a conversation the same way (origin kind "peer", the prompt wrapped in
// <agent-message from="…">); a task notification or a coordinator's note is the harness's and reads as nothing.

type queuedCommand struct {
	Type        string          `json:"type"`
	CommandMode string          `json:"commandMode"`
	Prompt      json.RawMessage `json:"prompt"`
	Origin      struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		From string `json:"from"`
		Body string `json:"body"`
	} `json:"origin"`
}

var agentMessageRe = regexp.MustCompile(`(?s)^\s*<agent-message[^>]*>(.*?)</agent-message>\s*$`)

// queuedOf reads an attachment record's message sent mid-turn: its text, and who sent it — "" for the person, a peer
// agent's name otherwise; false for any other attachment.
func queuedOf(raw json.RawMessage) (text, from string, ok bool) {
	if len(raw) == 0 {
		return "", "", false
	}
	var q queuedCommand
	if json.Unmarshal(raw, &q) != nil || q.Type != "queued_command" {
		return "", "", false
	}
	switch q.Origin.Kind {
	case "human", "":
		if q.CommandMode != "" && q.CommandMode != "prompt" {
			return "", "", false
		}
		text = promptText(q.Prompt)
	case "peer":
		from = q.Origin.Name
		if from == "" {
			from = q.Origin.From
		}
		if from == "" {
			from = "another agent"
		}
		text = q.Origin.Body
		if strings.TrimSpace(text) == "" {
			text = promptText(q.Prompt)
			if m := agentMessageRe.FindStringSubmatch(text); m != nil {
				text = m[1]
			}
		}
	default:
		return "", "", false
	}
	text = strings.TrimSpace(text)
	if text == "" || skipUserText(text) {
		return "", "", false
	}
	return userText(text), from, true
}

// promptText is a queued prompt as text: a string, or content blocks with a picture as "[image]".
func promptText(raw json.RawMessage) string {
	if s := stringContent(raw); s != "" {
		return s
	}
	var parts []string
	for _, b := range blocks(raw) {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image":
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}

// sentMidTurn files a message sent while the agent worked into the turn it was typed into, the last one, as a part of
// its own with its record; before any turn it opens one of the person's.
func sentMidTurn(turns []Turn, rec record, text, from string) []Turn {
	part := Part{Type: "queued", Text: text, At: rec.Timestamp, From: from}
	n := len(turns)
	if n == 0 {
		return append(turns, Turn{Role: "user", Ts: rec.Timestamp, IDs: []string{rec.UUID}, Parts: []Part{part}, ended: rec.Timestamp})
	}
	last := &turns[n-1]
	last.Parts = append(last.Parts, part)
	if rec.UUID != "" {
		last.IDs = append(last.IDs, rec.UUID)
	}
	return turns
}
