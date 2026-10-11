package health

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	out := `os=macOS 26.6.2
claude=2.1.285 (Claude Code)
claude_path=/opt/homebrew/bin/claude
herdr=herdr 0.9.3
herdr_path=/opt/homebrew/bin/herdr
arch=arm64
codex=codex-cli 0.153.0
codex_path=/opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js
herdr_ui=herdr-ui v0.4.0-8-g3262e05
df=/dev/disk3s5   482797652 188483048 271706504    41% 1511806 2717065040    0%   /System/Volumes/Data
uptime=23:21  up 33 mins, 2 users, load averages: 6.52 3.32 3.43
`
	r, peer := Parse(out)
	if r.OS != "macOS 26.6.2" || r.Claude != "2.1.285" || r.ClaudeVia != "brew" || r.Herdr != "0.9.3" || r.HerdrUI != "v0.4.0-8-g3262e05" {
		t.Fatalf("versions: %+v", r)
	}
	if r.HerdrVia != "brew" || r.Arch != "arm64" || r.Codex != "0.153.0" || r.CodexVia != "npm" {
		t.Fatalf("via/arch: %+v", r)
	}
	if r.DiskUsedPct != 41 || r.DiskFreeGB < 259 || r.DiskFreeGB > 260 {
		t.Fatalf("disk: %+v", r)
	}
	if r.Load1 != 6.52 || r.Uptime != "33 mins" {
		t.Fatalf("load/uptime: %+v", r)
	}
	// Linux: "up 5 days, 3:02, 1 user, load average: 0.52, 0.58, 0.59"
	l, _ := Parse("os=Linux 6.8.0\nuptime= 10:11:12 up 5 days,  3:02,  1 user,  load average: 0.52, 0.58, 0.59\ndf=/dev/sda1 100000000 50000000 50000000 50% /\n")
	if l.Load1 != 0.52 || l.Uptime != "5 days,  3:02" || l.DiskUsedPct != 50 {
		t.Fatalf("linux: %+v", l)
	}
	if e, _ := Parse(""); e.Claude != "" {
		t.Fatal("empty")
	}
	if peer != nil || r.Running {
		t.Fatal("no peer lines, no peer")
	}
	// a running herdr-ui with its token is a peer; a token alone is not
	p, peer2 := Parse("peer={\"herdr\":true,\"ok\":true,\"version\":\"v0.4.0-9-gabc\"}\npeer_token=abc123\n")
	if !p.Running || peer2 == nil || peer2.Token != "abc123" || peer2.Version != "v0.4.0-9-gabc" || peer2.Port != 7317 {
		t.Fatalf("peer: %+v %+v", p, peer2)
	}
	if _, none := Parse("peer_token=abc123\n"); none != nil {
		t.Fatal("a token without an answer is no peer")
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.1.285", "2.1.295", true},
		{"2.1.295", "2.1.285", false},
		{"0.9.3", "v0.9.3", false},
		{"0.9.1", "v0.9.3", true},
		{"", "1.0.0", false},
		{"1.9.0", "2.0.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestInstalledVia(t *testing.T) {
	if installedVia("../Cellar/claude-code/2.1.285/bin/claude") != "brew" || installedVia("/opt/homebrew/bin/claude") != "brew" || installedVia("../lib/node_modules/@anthropic-ai/claude-code/cli.js") != "npm" || installedVia("/Users/x/.local/bin/claude") != "native" || installedVia("/Users/x/.nvm/versions/node/v22/bin/claude") != "npm" || installedVia("/usr/bin/claude") != "" {
		t.Fatal("via")
	}
}

func TestPlan(t *testing.T) {
	r := &Report{Claude: "2.1.285", ClaudeVia: "brew", Codex: "0.160.0", CodexVia: "npm", Herdr: "0.9.1", HerdrVia: "installer"}
	if u, err := Plan("claude", r); err != nil || !strings.Contains(u.Command, "brew update && brew upgrade claude-code") {
		t.Fatalf("claude: %+v %v", u, err)
	}
	if u, err := Plan("codex", r); err != nil || !strings.Contains(u.Command, "@openai/codex@latest") {
		t.Fatalf("codex: %+v %v", u, err)
	}
	if u, err := Plan("herdr", r); err != nil || !strings.Contains(u.Command, "herdr update --handoff") {
		t.Fatalf("herdr: %+v %v", u, err)
	}
	if _, err := Plan("gemini", r); err == nil {
		t.Fatal("gemini is not installed")
	}
	if u, err := Plan("agents", r); err != nil || !strings.Contains(u.Command, "Update Claude Code") || strings.Contains(u.Command, "Gemini") {
		t.Fatalf("agents: %+v %v", u, err)
	}
	if u, _ := Plan("agents", r); strings.Count(u.Command, "brew update ||") != 1 || strings.Contains(u.Command, "brew update && brew upgrade") {
		t.Fatalf("agents: brew update once, up front: %s", u.Command)
	}
	if u, _ := Plan("herdr", &Report{Herdr: "0.9.3", HerdrVia: "brew"}); !strings.HasSuffix(u.Command, "brew update && brew upgrade herdr && herdr --version") {
		t.Fatalf("herdr brew: %s", u.Command)
	}
	ups := Updates(r, Latest{Claude: "2.1.296", Herdr: "0.9.3"})
	if len(ups) != 3 || !strings.Contains(ups[0].Note, "0.9.1 → 0.9.3") || !strings.Contains(ups[1].Note, "→ 2.1.296") {
		t.Fatalf("updates: %+v", ups)
	}
}
