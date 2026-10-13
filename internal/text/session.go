package text

import "regexp"

// sessionIDRe is what a session id may look like: it comes from the pane's own hooks (the agent chooses the value)
// and goes on to name a file and to be typed as `--resume <id>`, so nothing an option parser or a shell reads as more.
var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// SessionID says the value can be used as a file name and as a `--resume` argument as it is.
func SessionID(s string) bool { return sessionIDRe.MatchString(s) }
