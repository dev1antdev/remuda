// Package health reads each machine's vitals and the newest agent, herdr
// and remuda versions: the only requests remuda makes by itself.
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

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Report is one machine's vitals, as last read.
type Report struct {
	At time.Time `json:"at"`
	OS string    `json:"os,omitempty"` // "macOS 26.6.2", "Linux 6.8.0"

	Claude      string  `json:"claude,omitempty"`
	ClaudeVia   string  `json:"claude_via,omitempty"` // brew | npm | native | "" — how to update it
	Codex       string  `json:"codex,omitempty"`
	CodexVia    string  `json:"codex_via,omitempty"`
	Gemini      string  `json:"gemini,omitempty"`
	GeminiVia   string  `json:"gemini_via,omitempty"`
	HerdrVia    string  `json:"herdr_via,omitempty"` // brew | installer | "" — how herdr is updated
	Arch        string  `json:"arch,omitempty"`      // arm64 | amd64
	Herdr       string  `json:"herdr,omitempty"`
	HerdrServer string  `json:"herdr_server,omitempty"`
	Remuda      string  `json:"remuda,omitempty"`
	Running     bool    `json:"remuda_running"` // a remuda answers on the machine's own port: terminals go through it
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
	Codex  string    `json:"codex,omitempty"`
	Gemini string    `json:"gemini,omitempty"`
	Herdr  string    `json:"herdr,omitempty"`
	Remuda string    `json:"remuda,omitempty"`
}

// Collector keeps the reports and the latest versions fresh.
type Collector struct {
	machines *machines.Manager
	// Version is this server's own, for the "remuda there is older" hint
	Version string
	// Check turns the outbound version lookups on
	Check bool

	mu      sync.Mutex
	reports map[string]*Report
	latest  Latest
	release Release
	locks   map[string]*sync.Mutex // one read of a machine at a time; a second waits for it
}

func New(m *machines.Manager, version string, check bool) *Collector {
	return &Collector{machines: m, Version: version, Check: check, reports: map[string]*Report{}, locks: map[string]*sync.Mutex{}}
}

// Start keeps every connected machine's report no older than interval and the versions fresh.
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

// Latest is the newest known Claude Code, herdr and remuda.
func (c *Collector) Latest() Latest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest
}

// Release is the newest release of remuda as last read, with its assets.
func (c *Collector) Release() Release {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.release
}

// RefreshLatest looks the versions up again when the last look is older than maxAge.
func (c *Collector) RefreshLatest(maxAge time.Duration) {
	if !c.Check {
		return
	}
	c.mu.Lock()
	old := time.Since(c.latest.At) >= maxAge
	c.mu.Unlock()
	if old {
		c.fetchLatest()
	}
}

// Refresh reads one machine now; a read already running is waited for.
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
	out, err := shell.Text(mc, Script(), 30*time.Second)
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

// Script prints the vitals, one "key=value" per line, every probe silent when its tool is missing.
// PATH is set first: a non-interactive shell over ssh starts without the package managers' bin dirs.
func Script() string {
	return script.PATH + `
LC_ALL=C; export LC_ALL
if command -v sw_vers >/dev/null 2>&1; then echo "os=macOS $(sw_vers -productVersion 2>/dev/null)"; else echo "os=$(uname -s) $(uname -r)"; fi
for a in claude codex gemini; do c=$(command -v "$a" 2>/dev/null); if [ -n "$c" ]; then echo "$a=$("$a" --version 2>/dev/null | head -1)"; echo "${a}_path=$(readlink "$c" 2>/dev/null || printf %s "$c")"; fi; done
h=$(command -v herdr 2>/dev/null); if [ -n "$h" ]; then echo "herdr=$(herdr --version 2>/dev/null | head -1)"; echo "herdr_path=$(readlink "$h" 2>/dev/null || printf %s "$h")"; echo "herdr_server=$(herdr status server 2>/dev/null | sed -n 's/^version: //p' | head -1)"; fi
echo "arch=$(uname -m)"
if [ -x "$HOME/.local/bin/remuda" ]; then echo "remuda=$("$HOME/.local/bin/remuda" -version 2>/dev/null)"; fi
echo "df=$(df -kP "$HOME" 2>/dev/null | tail -1)"
echo "uptime=$(uptime 2>/dev/null)"
tf=""; for f in "$HOME/Library/Application Support/remuda/token" "$HOME/.config/remuda/token"; do [ -r "$f" ] && { tf="$f"; break; }; done
if [ -n "$tf" ] && command -v curl >/dev/null 2>&1; then ph=$(curl -s -m 2 http://127.0.0.1:7317/api/health 2>/dev/null); case "$ph" in *'"ok":true'*) echo "peer=$ph"; echo "peer_token=$(cat "$tf")";; esac; fi
`
}

var (
	semver   = regexp.MustCompile(`\d+\.\d+\.\d+`)
	loadRe   = regexp.MustCompile(`load averages?: *([\d.]+)`)
	upRe     = regexp.MustCompile(`up (.*?),\s+\d+ users?`)
	fieldsRe = regexp.MustCompile(`\s+`)
)

// Parse reads the script's output: the report and, when one answered, the peer remuda.
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
		case "codex":
			r.Codex = semver.FindString(v)
		case "codex_path":
			r.CodexVia = installedVia(v)
		case "gemini":
			r.Gemini = semver.FindString(v)
		case "gemini_path":
			r.GeminiVia = installedVia(v)
		case "herdr":
			r.Herdr = semver.FindString(v)
		case "herdr_server":
			r.HerdrServer = semver.FindString(v)
		case "herdr_path":
			if installedVia(v) == "brew" {
				r.HerdrVia = "brew"
			} else {
				r.HerdrVia = "installer"
			}
		case "arch":
			switch v {
			case "arm64", "aarch64":
				r.Arch = "arm64"
			case "x86_64", "amd64":
				r.Arch = "amd64"
			default:
				r.Arch = v
			}
		case "remuda":
			r.Remuda = strings.TrimPrefix(v, "remuda ")
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
		peer = nil
	}
	return r, peer
}

// installedVia guesses the package manager from where the binary really lives (the symlink read).
// An npm global under Homebrew's node is a symlink in /opt/homebrew/bin into node_modules: npm, not brew.
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

func (c *Collector) refreshLatest() {
	c.mu.Lock()
	fresh := time.Since(c.latest.At) < 6*time.Hour
	c.mu.Unlock()
	if !fresh {
		c.fetchLatest()
	}
}

// fetchLatest asks npm and GitHub now; a lookup that fails keeps the last known value.
func (c *Collector) fetchLatest() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var l Latest
	l.At = time.Now()
	if v, err := fetchJSON(ctx, "https://registry.npmjs.org/@anthropic-ai/claude-code/latest", "version"); err == nil {
		l.Claude = semver.FindString(v)
	} else {
		log.Printf("health: claude code latest: %v", err)
	}
	for _, a := range []struct {
		pkg string
		dst *string
	}{{"@openai/codex", &l.Codex}, {"@google/gemini-cli", &l.Gemini}} {
		if v, err := fetchJSON(ctx, "https://registry.npmjs.org/"+a.pkg+"/latest", "version"); err == nil {
			*a.dst = semver.FindString(v)
		} else {
			log.Printf("health: %s latest: %v", a.pkg, err)
		}
	}
	if v, err := fetchJSON(ctx, "https://api.github.com/repos/herdrdev/herdr/releases/latest", "tag_name"); err == nil {
		l.Herdr = semver.FindString(v)
	} else {
		log.Printf("health: herdr latest: %v", err)
	}
	var rel Release
	if r, err := fetchRelease(ctx, releasesURL); err == nil {
		rel, l.Remuda = r, r.Version
	} else {
		log.Printf("health: remuda latest: %v", err)
	}
	c.mu.Lock()
	if l.Claude == "" {
		l.Claude = c.latest.Claude
	}
	if l.Herdr == "" {
		l.Herdr = c.latest.Herdr
	}
	if l.Codex == "" {
		l.Codex = c.latest.Codex
	}
	if l.Gemini == "" {
		l.Gemini = c.latest.Gemini
	}
	if l.Remuda == "" {
		l.Remuda = c.latest.Remuda
	} else {
		c.release = rel
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
	req.Header.Set("User-Agent", "remuda")
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
