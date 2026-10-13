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
herdr_server=0.9.1
arch=arm64
codex=codex-cli 0.153.0
codex_path=/opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js
remuda=remuda v0.4.0-8-g3262e05
df=/dev/disk3s5   482797652 188483048 271706504    41% 1511806 2717065040    0%   /System/Volumes/Data
uptime=23:21  up 33 mins, 2 users, load averages: 6.52 3.32 3.43
`
	r, peer := Parse(out)
	if r.OS != "macOS 26.6.2" || r.Claude != "2.1.285" || r.ClaudeVia != "brew" || r.Herdr != "0.9.3" || r.Remuda != "v0.4.0-8-g3262e05" {
		t.Fatalf("versions: %+v", r)
	}
	if r.HerdrVia != "brew" || r.Arch != "arm64" || r.Codex != "0.153.0" || r.CodexVia != "npm" || r.HerdrServer != "0.9.1" {
		t.Fatalf("via/arch: %+v", r)
	}
	if r.DiskUsedPct != 41 || r.DiskFreeGB < 259 || r.DiskFreeGB > 260 {
		t.Fatalf("disk: %+v", r)
	}
	if r.Load1 != 6.52 || r.Uptime != "33 mins" {
		t.Fatalf("load/uptime: %+v", r)
	}
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

// ---- remuda's own releases ----

func TestNewerReadsTheVersionOutOfADescribe(t *testing.T) {
	if !Newer("v0.4.0-24-g5dac31e", "v0.5.0") {
		t.Fatal("a release past the tag the build sits on is newer")
	}
	if Newer("v0.4.0-24-g5dac31e", "v0.4.0") {
		t.Fatal("the tag the build sits on is not newer than the build")
	}
	if Newer("dev", "v0.5.0") || Newer("v0.5.0", "") {
		t.Fatal("no version, no comparison")
	}
}

const releaseJSON = `{"tag_name":"v0.5.0","name":"remuda 0.5.0","assets":[
 {"name":"remuda-darwin-arm64","browser_download_url":"https://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-darwin-arm64"},
 {"name":"remuda-linux-amd64","browser_download_url":"https://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-linux-amd64"},
 {"name":"checksums.txt","browser_download_url":"https://github.com/dev1antdev/remuda/releases/download/v0.5.0/checksums.txt"}]}`

func TestParseRelease(t *testing.T) {
	rel, err := parseRelease(strings.NewReader(releaseJSON))
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.5.0" || rel.Version != "0.5.0" || len(rel.Assets) != 2 || !strings.HasSuffix(rel.Assets["remuda-darwin-arm64"], "/v0.5.0/remuda-darwin-arm64") || !strings.HasSuffix(rel.Checksums, "/checksums.txt") {
		t.Fatalf("release %+v", rel)
	}
	if _, err := parseRelease(strings.NewReader(`{"tag_name":"nightly","assets":[]}`)); err == nil {
		t.Fatal("a tag without a version was accepted")
	}
}
