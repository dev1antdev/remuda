package prstatus

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// A GitHub pull request is probed before it is read: one conditional REST request for the pull request with the ETag
// of the last answer, which GitHub answers 304 at no cost to the rate limit while nothing moved. A 304 keeps the last
// full read while it still stands — checks running move without the pull request moving, so that read stands 2 minutes;
// an open request's 10; a closed or merged one's an hour — and anything else reads the request in full again.

// standFor is how long a full read of a request in this state stands while its probe says nothing moved.
func standFor(s *Status) time.Duration {
	switch {
	case s == nil:
		return 0
	case s.State == "OPEN" && s.Checks.Pending > 0:
		return 2 * time.Minute
	case s.State == "OPEN":
		return 10 * time.Minute
	}
	return time.Hour
}

// ProbeScript asks GitHub for the pull request's headers alone, with the ETag when there is one.
func ProbeScript(url, etag string) string {
	owner, repo, number, ok := URLParts(url)
	if !ok {
		return ""
	}
	call := "gh api -i"
	if etag != "" {
		call += " -H " + script.Quote("If-None-Match: "+etag)
	}
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, number)
	return script.PATH + "\n" + call + " " + script.Quote(path) + ` 2>&1 | awk '{ sub(/\r$/, "") } $0 == "" { exit } { print }'`
}

var statusLine = regexp.MustCompile(`^HTTP/\S+ (\d{3})`)

// ParseProbe reads the status and the ETag off the headers gh printed; 0 when gh printed none.
func ParseProbe(out string) (code int, etag string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if m := statusLine.FindStringSubmatch(line); m != nil && code == 0 {
			code, _ = strconv.Atoi(m[1])
			continue
		}
		if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "etag") {
			etag = strings.TrimSpace(value)
		}
	}
	return code, etag
}

// probe is one conditional request on the machine.
func probe(host shell.Shell, url, etag string) (int, string) {
	s := ProbeScript(url, etag)
	if s == "" {
		return 0, ""
	}
	out, _ := shell.Text(host, s, Timeout)
	return ParseProbe(out)
}
