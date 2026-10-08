// Package health reads each machine's vital signs — which Claude Code,
// herdr and herdr-ui it runs, how much disk is left, how loaded it is —
// and what the newest Claude Code and herdr are, so the sidebar can say
// when a machine wants an update or is running out of room.
//
// One POSIX script per machine, a few minutes apart, through the same
// shell the rest of herdr-ui uses (local exec or the SSH master). The
// latest versions come from the npm registry and GitHub's releases API,
// once every few hours, and only when version checks are on: those two
// requests are the only thing herdr-ui ever sends anywhere.
package health

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/machines"
)

// Report is one machine's vitals, as last read.
type Report struct {
	At time.Time `json:"at"`
	OS string    `json:"os,omitempty"` // "macOS 26.6.2", "Linux 6.8.0"

	Claude      string  `json:"claude,omitempty"`     // installed Claude Code version
	ClaudeVia   string  `json:"claude_via,omitempty"` // brew | npm | native | "" — how to update it
	Herdr       string  `json:"herdr,omitempty"`      // installed herdr version
	HerdrUI     string  `json:"herdr_ui,omitempty"`   // the herdr-ui binary deployed there (remote machines)
	Running     bool    `json:"herdr_ui_running"`     // a herdr-ui answers on the machine\'s own port: terminals go through it
	DiskFreeGB  float64 `json:"disk_free_gb"`
	DiskUsedPct int     `json:"disk_used_pct"`
	Load1       float64 `json:"load1"`
	Uptime      string  `json:"uptime,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// Latest is what the world offers right now.
type Latest struct {
	At     time.Time `json:"at"`
	Claude string    `json:"claude,omitempty"`
	Herdr  string    `json:"herdr,omitempty"`
}

// Collector keeps the reports and the latest versions fresh.
type Collector struct {
	machines *machines.Manager
	// Version is this server's own, for the "herdr-ui there is older" hint
	Version string
	// Check turns the outbound version lookups on
	Check bool

	mu      sync.Mutex
	reports map[string]*Report
	latest  Latest
	locks   map[string]*sync.Mutex // one read of a machine at a time; a second waits for it
}

func New(m *machines.Manager, version string, check bool) *Collector {
	return &Collector{machines: m, Version: version, Check: check, reports: map[string]*Report{}, locks: map[string]*sync.Mutex{}}
}

// Start keeps every connected machine's report no older than interval —
// a machine that connects later is read within a minute of it — and the
// latest versions a few hours fresh.
func (c *Collector) Start(interval time.Duration) {
	go func() {
		for {
			if c.Check {
				c.refreshLatest()
			}
			for _, mc := range c.machines.List() {
				if st, _ := mc.State(); st != machines.StateConnected {
					continue
				}
				c.mu.Lock()
				r := c.reports[mc.Cfg.ID]
				c.mu.Unlock()
				if r == nil || time.Since(r.At) >= interval {
					c.Refresh(mc)
				}
			}
			time.Sleep(time.Minute)
		}
	}()
}

// Report is the machine's last reading, nil before the first.
func (c *Collector) Report(id string) *Report {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.reports[id]
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

// Latest is the newest known Claude Code and herdr.
func (c *Collector) Latest() Latest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest
}

// Refresh reads one machine now; a read already running is waited for,
// so a caller always returns with a fresh report.
func (c *Collector) Refresh(mc *machines.Machine) {
	c.mu.Lock()
	l := c.locks[mc.Cfg.ID]
	if l == nil {
		l = &sync.Mutex{}
		c.locks[mc.Cfg.ID] = l
	}
	c.mu.Unlock()
	l.Lock()
	defer l.Unlock()
	out, err := mc.Shell(Script(), 30*time.Second)
	r, peer := Parse(out)
	r.At = time.Now()
	if err != nil && r.Claude == "" && r.Herdr == "" {
		r.Error = err.Error()
	}
	if peer != nil {
		peer.At = r.At
	}
	if err == nil || peer != nil {
		mc.SetPeer(peer) // a probe that failed outright says nothing about the peer
	}
	c.mu.Lock()
	c.reports[mc.Cfg.ID] = &r
	c.mu.Unlock()
}

// Script prints the vitals, one "key=value" per line. Every probe is
// optional and silent when a tool is missing; PATH takes in the places
// package managers put binaries, since a non-interactive shell over SSH
// starts without them.
func Script() string {
	return `PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"; export PATH
LC_ALL=C; export LC_ALL
if command -v sw_vers >/dev/null 2>&1; then echo "os=macOS $(sw_vers -productVersion 2>/dev/null)"; else echo "os=$(uname -s) $(uname -r)"; fi
c=$(command -v claude 2>/dev/null); if [ -n "$c" ]; then echo "claude=$(claude --version 2>/dev/null | head -1)"; echo "claude_path=$(readlink "$c" 2>/dev/null || printf %s "$c")"; fi
if command -v herdr >/dev/null 2>&1; then echo "herdr=$(herdr --version 2>/dev/null | head -1)"; fi
if [ -x "$HOME/.local/bin/herdr-ui" ]; then echo "herdr_ui=$("$HOME/.local/bin/herdr-ui" -version 2>/dev/null)"; fi
echo "df=$(df -kP "$HOME" 2>/dev/null | tail -1)"
echo "uptime=$(uptime 2>/dev/null)"
tf=""; for f in "$HOME/Library/Application Support/herdr-ui/token" "$HOME/.config/herdr-ui/token"; do [ -r "$f" ] && { tf="$f"; break; }; done
if [ -n "$tf" ] && command -v curl >/dev/null 2>&1; then ph=$(curl -s -m 2 http://127.0.0.1:7317/api/health 2>/dev/null); case "$ph" in *'"ok":true'*) echo "peer=$ph"; echo "peer_token=$(cat "$tf")";; esac; fi
`
}

var (
	semver   = regexp.MustCompile(`\d+\.\d+\.\d+`)
	loadRe   = regexp.MustCompile(`load averages?: *([\d.]+)`)
	upRe     = regexp.MustCompile(`up (.*?),\s+\d+ users?`)
	fieldsRe = regexp.MustCompile(`\s+`)
)

// Parse reads the script's output: the report, and the herdr-ui running
// there when the script found one (its health answer and its token).
func Parse(out string) (Report, *machines.Peer) {
	var r Report
	var peer *machines.Peer
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "os":
			r.OS = strings.TrimSpace(v)
		case "claude":
			r.Claude = semver.FindString(v)
		case "claude_path":
			r.ClaudeVia = installedVia(v)
		case "herdr":
			r.Herdr = semver.FindString(v)
		case "herdr_ui":
			r.HerdrUI = strings.TrimPrefix(v, "herdr-ui ")
		case "df":
			// Filesystem 1K-blocks Used Available Use% ... (macOS adds iused columns)
			f := fieldsRe.Split(v, -1)
			if len(f) >= 5 {
				if avail, err := strconv.ParseFloat(f[3], 64); err == nil {
					r.DiskFreeGB = avail / (1024 * 1024)
				}
				if pct, err := strconv.Atoi(strings.TrimSuffix(f[4], "%")); err == nil {
					r.DiskUsedPct = pct
				}
			}
		case "uptime":
			if m := loadRe.FindStringSubmatch(v); m != nil {
				r.Load1, _ = strconv.ParseFloat(m[1], 64)
			}
			if m := upRe.FindStringSubmatch(v); m != nil {
				r.Uptime = strings.TrimSpace(m[1])
			}
		case "peer":
			var ph struct {
				OK      bool   `json:"ok"`
				Version string `json:"version"`
			}
			if json.Unmarshal([]byte(v), &ph) == nil && ph.OK {
				r.Running = true
				if peer == nil {
					peer = &machines.Peer{Port: 7317}
				}
				peer.Version = ph.Version
			}
		case "peer_token":
			if peer == nil {
				peer = &machines.Peer{Port: 7317}
			}
			peer.Token = v
		}
	}
	if peer != nil && (peer.Token == "" || !r.Running) {
		peer = nil // half a peer is none: no token to speak with, or no answer
	}
	return r, peer
}

// installedVia guesses the package manager from where the binary really
// lives (the symlink read), so the hint names the right update command:
// an npm global under Homebrew's node is a symlink in /opt/homebrew/bin
// into node_modules — npm, not brew.
func installedVia(path string) string {
	switch {
	case strings.Contains(path, "node_modules") || strings.Contains(path, "/.npm") || strings.Contains(path, "/.nvm/") || strings.Contains(path, "/.volta/") || strings.Contains(path, "/.bun/"):
		return "npm"
	case strings.Contains(path, "/homebrew/") || strings.Contains(path, "/Cellar/"):
		return "brew"
	case strings.Contains(path, "/.local/bin/") || strings.Contains(path, "/.local/share/claude/"):
		return "native"
	}
	return ""
}

// UpdateHint is the command that updates Claude Code installed that way.
func UpdateHint(via string) string {
	switch via {
	case "brew":
		return "brew upgrade claude-code"
	case "npm":
		return "npm i -g @anthropic-ai/claude-code"
	default:
		return "claude update"
	}
}

func (c *Collector) refreshLatest() {
	c.mu.Lock()
	fresh := time.Since(c.latest.At) < 6*time.Hour
	c.mu.Unlock()
	if fresh {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var l Latest
	l.At = time.Now()
	if v, err := fetchJSON(ctx, "https://registry.npmjs.org/@anthropic-ai/claude-code/latest", "version"); err == nil {
		l.Claude = semver.FindString(v)
	} else {
		log.Printf("health: claude code latest: %v", err)
	}
	if v, err := fetchJSON(ctx, "https://api.github.com/repos/herdrdev/herdr/releases/latest", "tag_name"); err == nil {
		l.Herdr = semver.FindString(v)
	} else {
		log.Printf("health: herdr latest: %v", err)
	}
	c.mu.Lock()
	// a lookup that failed keeps the last known value
	if l.Claude == "" {
		l.Claude = c.latest.Claude
	}
	if l.Herdr == "" {
		l.Herdr = c.latest.Herdr
	}
	c.latest = l
	c.mu.Unlock()
}

func fetchJSON(ctx context.Context, url, field string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "herdr-ui")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", &httpError{url, res.StatusCode}
	}
	var m map[string]any
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return "", err
	}
	s, _ := m[field].(string)
	return s, nil
}

type httpError struct {
	url  string
	code int
}

func (e *httpError) Error() string { return e.url + ": HTTP " + strconv.Itoa(e.code) }

// Newer reports whether b is a newer semantic version than a.
func Newer(a, b string) bool {
	pa, pb := parts(a), parts(b)
	if pa == nil || pb == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if pb[i] != pa[i] {
			return pb[i] > pa[i]
		}
	}
	return false
}

func parts(v string) []int {
	v = semver.FindString(v)
	if v == "" {
		return nil
	}
	out := make([]int, 3)
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}
