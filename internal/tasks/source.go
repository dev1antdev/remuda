package tasks

import "strings"

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
	if len(out) > MaxSource {
		out = out[:MaxSource]
	}
	return out
}
