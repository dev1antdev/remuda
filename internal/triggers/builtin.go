package triggers

import (
	"regexp"
	"strconv"
	"strings"
)

// The built-in watches every pane gets, by their keys: a dev server saying where it serves, and tests that failed.
const (
	WatchServe = "serve"
	WatchTests = "tests"
	rulePrefix = "rule:"
)

// lines is how many of a pane's last rows herdr reads for a watch: a test run's summary and a dev server's banner sit
// at the bottom, and herdr tells a match the moment it appears in them, again only once it has left them.
const lines = 40

// ServePattern is a dev server announcing its address, in the words of Vite, Next.js, Create React App, Astro,
// webpack-dev-server, python -m http.server, uvicorn, Flask, Django, Rails, Hugo, Jekyll, php -S and a plain
// "listening on :3000". Read by herdr's Rust regex and Go's alike.
const ServePattern = `\bLocal:?\s+https?://|\bLoopback:\s+https?://|Serving HTTP on \S+ port \d+|Uvicorn running on https?://` +
	`|\bRunning on https?://|development server at https?://|(?i:listening on)\s+(https?://|\S*:\d{2,5}\b|port\s+\d{2,5}\b)` +
	`|(?i:server (is )?(running|listening|started) (at|on))\s+(https?://|port\s+\d)|ready - started server on` +
	`|Web Server is available at https?://|Server address: https?://|Development Server \(https?://`

// TestsPattern is a test run that failed, as go test, jest, vitest, pytest, cargo test, mocha, rspec and node's own
// runner sum it up. A tab in a terminal is spaces by the time herdr reads the rows, so go's "FAIL\tpkg" is FAIL and
// spaces.
const TestsPattern = `^(--- FAIL: |FAIL\s+\S+\s+(\d|\[)|Tests:\s+\d+ failed|\s*Tests?\s+(Files\s+)?\d+ failed|=+ .*\b\d+ failed\b` +
	`|FAILED \S+::|test result: FAILED|\s+\d+ failing\s*$|\d+ examples?, [1-9]\d* failures?|# fail [1-9])`

var (
	serveRe = regexp.MustCompile(ServePattern)
	testsRe = regexp.MustCompile(TestsPattern)
	urlRe   = regexp.MustCompile(`https?://(\[[0-9a-fA-F:]+\]|[^\s/:\[\]]+)(:(\d{1,5}))?[^\s]*`)
	portRe  = regexp.MustCompile(`(?i)\bport\s+(\d{2,5})\b`)
	colonRe = regexp.MustCompile(`:(\d{2,5})\b`)
)

// Serves says whether a row is a dev server announcing itself.
func Serves(line string) bool { return serveRe.MatchString(line) }

// TestsFailed says whether a row sums up a failed test run.
func TestsFailed(line string) bool { return testsRe.MatchString(line) }

// ServedAt reads the port a row names — its address's, else "port 8000", else ":3000" — and the address as printed;
// a port under 1024 is none, as the previews never open one.
func ServedAt(line string) (port int, url string, ok bool) {
	if m := urlRe.FindStringSubmatch(line); m != nil {
		url = strings.TrimRight(m[0], ".,;:)]'\"")
		if m[3] != "" {
			port, _ = strconv.Atoi(m[3])
		}
	}
	if port == 0 {
		if m := portRe.FindStringSubmatch(line); m != nil {
			port, _ = strconv.Atoi(m[1])
		} else if m := colonRe.FindStringSubmatch(line); m != nil {
			port, _ = strconv.Atoi(m[1])
		}
	}
	if port < 1024 || port > 65535 {
		return 0, url, false
	}
	return port, url, true
}
