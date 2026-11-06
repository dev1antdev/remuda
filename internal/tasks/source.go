package tasks

import (
	"strings"
	"unicode/utf8"
)

// MaxSource bounds what a task says about its maker.
const MaxSource = 120

// Source names who made a task: "" for a person, whatever the body claimed; for an agent key the key's name (the
// machine it was placed on) and, after a middle dot, what the caller said of itself, its pane.
func Source(caller, said string) string {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return ""
	}
	out := caller
	if said = strings.TrimSpace(said); said != "" {
		out += " · " + said
	}
	return cutBytes(out, MaxSource)
}

// cutBytes keeps at most max bytes of s, ending between characters rather than inside one.
func cutBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}
