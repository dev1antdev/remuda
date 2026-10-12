package text

import "regexp"

// A session id as herdr reports it comes from the pane's own hooks — the
// agent in the pane chooses the value — and goes on to name a file and to
// be typed as `--resume <id>`. So it may look like one (Claude Code's are
// UUIDs, Codex's and Gemini's similar) and like nothing an option parser
// or a shell would read as more.
var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// SessionID says the value can be used as a file name and as a `--resume`
// argument as it is.
func SessionID(s string) bool { return sessionIDRe.MatchString(s) }
