package chat

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Claude Code's spinner sits just above the input box while the agent works: a glyph, the verb, and in brackets how long
// the turn has run and the tokens it has written — "✻ Refactoring… (2m 14s · ↓ 6.8k tokens · thinking)", the words
// read off 2.1.323's own spinner (the elapsed time as its formatDuration writes it, the tokens in compact notation,
// lower-cased, ↑ while a request goes out). Early in a turn the brackets are not there yet; a narrow pane drops their
// parts from the right; an older one ends them with "esc to interrupt".

// Progress is what the spinner says: the verb, the turn's time by the spinner's clock, the tokens written so far.
type Progress struct {
	Verb    string `json:"verb"`
	Seconds int    `json:"seconds,omitempty"`
	Tokens  int64  `json:"tokens,omitempty"`
}

const (
	spinnerGlyphs = "·✢✳✶✻✽*"
	ellipsis      = "…"
	progressRows  = 20 // how far above the input box the spinner is looked for: the plan's items and a tip sit between
	inputRows     = 16 // how many rows a draft in the input box may take
)

var (
	durationRe = regexp.MustCompile(`^(?:(\d+)d\s*)?(?:(\d+)h\s*)?(?:(\d+)m\s*)?(?:(\d+)(?:\.\d+)?s)?$`)
	tokensRe   = regexp.MustCompile(`(?i)^\D{0,3}?([\d.,]+)\s*([kmb]?)\s*tokens$`)
)

// ParseProgress reads the spinner off a pane's visible screen: only in the rows over the input box, so a line of the
// conversation never counts; false when no spinner stands there (the agent at rest, a dialog over the box, another agent).
func ParseProgress(screen string) (Progress, bool) {
	rows := strings.Split(strings.ReplaceAll(screen, "\r\n", "\n"), "\n")
	top := inputBoxTop(rows)
	if top < 0 {
		return Progress{}, false
	}
	for i := top - 1; i >= 0 && top-i <= progressRows; i-- {
		if p, ok := spinnerRow(rows[i]); ok {
			return p, true
		}
	}
	return Progress{}, false
}

// inputBoxTop is the row of the input box's top edge: from the bottom, the first edge found is its bottom, the next one
// up within inputRows its top; -1 when the screen shows no box.
func inputBoxTop(rows []string) int {
	bottom := -1
	for i := len(rows) - 1; i >= 0; i-- {
		if !boxEdge(rows[i]) {
			continue
		}
		if bottom < 0 {
			bottom = i
			continue
		}
		if bottom-i <= inputRows {
			return i
		}
		return -1
	}
	return -1
}

// boxEdge is a solid rule, or a rounded box's top or bottom edge.
func boxEdge(row string) bool {
	trimmed := strings.TrimSpace(row)
	for _, ends := range []string{"╭╮", "╰╯"} {
		corners := []rune(ends)
		if strings.HasPrefix(trimmed, string(corners[0])) && strings.HasSuffix(trimmed, string(corners[1])) {
			trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, string(corners[0])), string(corners[1]))
			break
		}
	}
	return len([]rune(trimmed)) >= 8 && strings.Trim(trimmed, ruleRunes) == ""
}

// spinnerRow reads one row as the spinner: a glyph and a space, a verb ending in an ellipsis, the brackets after it.
func spinnerRow(row string) (Progress, bool) {
	line := strings.TrimSpace(row)
	glyph, size := utf8.DecodeRuneInString(line)
	if line == "" || !strings.ContainsRune(spinnerGlyphs, glyph) || !strings.HasPrefix(line[size:], " ") {
		return Progress{}, false
	}
	rest := strings.TrimSpace(line[size:])
	at := strings.Index(rest, ellipsis)
	if at <= 0 {
		return Progress{}, false
	}
	p := Progress{Verb: strings.TrimSpace(rest[:at])}
	if strings.ContainsAny(p.Verb, "()") {
		return Progress{}, false
	}
	after := strings.TrimSpace(rest[at+len(ellipsis):])
	if after == "" {
		return p, true
	}
	if !strings.HasPrefix(after, "(") {
		return Progress{}, false
	}
	inner := strings.TrimPrefix(after, "(")
	if end := strings.LastIndex(inner, ")"); end >= 0 {
		inner = inner[:end]
	}
	for _, segment := range strings.Split(inner, "·") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if seconds, ok := durationOf(segment); ok && p.Seconds == 0 {
			p.Seconds = seconds
		} else if tokens, ok := tokensOf(segment); ok && p.Tokens == 0 {
			p.Tokens = tokens
		}
	}
	return p, true
}

// durationOf reads "45s", "2m 14s", "1h 0m 5s", "1d 2h 3m" as seconds.
func durationOf(segment string) (int, bool) {
	m := durationRe.FindStringSubmatch(segment)
	if m == nil || segment == "" {
		return 0, false
	}
	unit := []int{86400, 3600, 60, 1}
	seconds := 0
	for i, group := range m[1:] {
		if group != "" {
			n, _ := strconv.Atoi(group)
			seconds += n * unit[i]
		}
	}
	return seconds, true
}

// tokensOf reads "↓ 6.8k tokens", "↑ 840 tokens", "↓ 1.2m tokens" as a count.
func tokensOf(segment string) (int64, bool) {
	m := tokensRe.FindStringSubmatch(segment)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(m[2]) {
	case "k":
		n *= 1e3
	case "m":
		n *= 1e6
	case "b":
		n *= 1e9
	}
	return int64(n + 0.5), true
}
