// Package sent words what a device typed into an agent through remuda — a text, a key, a chosen answer, the restart's
// own commands — as the quiet row the activity keeps for it: who sent what where. Never news: the bell stays dark.
package sent

import (
	"strings"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/feed"
)

// Kind is the activity kind of these rows.
const Kind = "sent"

// MaxText is how much of a text a row quotes.
const MaxText = 80

// Remuda is the sender of what remuda types by itself — the restart's exit and resume.
const Remuda = "remuda"

// APIClient is the sender behind a bearer token with no device of its own.
const APIClient = "an API client"

// keyNames are the words for herdr's key grammar as a person reads them.
var keyNames = map[string]string{
	"enter": "Enter", "esc": "Esc", "tab": "Tab", "shift+tab": "⇧Tab", "space": "Space",
	"up": "↑", "down": "↓", "left": "←", "right": "→",
	"home": "Home", "end": "End", "pageup": "PgUp", "pagedown": "PgDn",
	"backspace": "⌫", "delete": "Del",
	"ctrl+c": "Ctrl+C", "ctrl+d": "Ctrl+D", "ctrl+u": "Ctrl+U", "ctrl+z": "Ctrl+Z",
	"ctrl+r": "Ctrl+R", "ctrl+l": "Ctrl+L", "ctrl+k": "Ctrl+K",
}

// Entry is the activity row for one sending: "iPhone sent «yes» to api · w3:p1", keys bare, a text quoted and cut.
func Entry(device, text string, keys bool, machineID, paneID, workspace string) feed.Entry {
	what := Cut(text)
	if !keys {
		what = "«" + what + "»"
	}
	where := paneID
	if workspace != "" {
		where = workspace + " · " + paneID
	}
	return feed.Entry{Kind: Kind, Quiet: true, Machine: machineID, Pane: paneID, Title: device + " sent " + what + " to " + where}
}

// KeyWords names the keys as a person reads them, in the order pressed.
func KeyWords(keys []string) string {
	words := make([]string, 0, len(keys))
	for _, key := range keys {
		if name, ok := keyNames[strings.ToLower(key)]; ok {
			words = append(words, name)
		} else {
			words = append(words, key)
		}
	}
	return strings.Join(words, " ")
}

// Cut makes a text one line of at most MaxText runes, an ellipsis standing for the rest.
func Cut(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= MaxText {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:MaxText])) + "…"
}
