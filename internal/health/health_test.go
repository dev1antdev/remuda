package health

import (
	"os"
	"os/exec"
	"path/filepath"
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
kimi=2.1.1
kimi_path=/Users/x/.kimi-code/bin/kimi
remuda=remuda v0.4.0-8-g3262e05
hook=claude v10 < v11
hook=codex v8 < v10
plugin=herdr.push dcolinmorgan/herdr-push
df=/dev/disk3s5   482797652 188483048 271706504    41% 1511806 2717065040    0%   /System/Volumes/Data
uptime=23:21  up 33 mins, 2 users, load averages: 6.52 3.32 3.43
`
	r, peer := Parse(out)
	if r.OS != "macOS 26.6.2" || r.Claude != "2.1.285" || r.ClaudeVia != "brew" || r.Herdr != "0.9.3" || r.Remuda != "v0.4.0-8-g3262e05" {
		t.Fatalf("versions: %+v", r)
	}
	if r.HerdrVia != "brew" || r.Arch != "arm64" || r.Codex != "0.153.0" || r.CodexVia != "npm" || r.HerdrServer != "0.9.1" || r.Kimi != "2.1.1" || r.KimiVia != "native" {
		t.Fatalf("via/arch: %+v", r)
	}
	if r.DiskUsedPct != 41 || r.DiskFreeGB < 259 || r.DiskFreeGB > 260 {
		t.Fatalf("disk: %+v", r)
	}
	if r.Load1 != 6.52 || r.Uptime != "33 mins" {
		t.Fatalf("load/uptime: %+v", r)
	}
	if len(r.OutdatedHooks) != 2 || r.OutdatedHooks[0] != (Hook{"claude", "v10", "v11"}) || r.OutdatedHooks[1] != (Hook{"codex", "v8", "v10"}) {
		t.Fatalf("hooks: %+v", r.OutdatedHooks)
	}
	if len(r.Plugins) != 1 || r.Plugins[0] != (Plugin{"herdr.push", "dcolinmorgan/herdr-push"}) {
		t.Fatalf("plugins: %+v", r.Plugins)
	}
	if bad, _ := Parse("hook=$(id) v1 < v2\nhook=claude\nplugin=only-an-id\n"); len(bad.OutdatedHooks) != 1 || bad.OutdatedHooks[0] != (Hook{Agent: "claude"}) || len(bad.Plugins) != 0 {
		t.Fatalf("a hook line is an agent name and maybe its versions, a plugin line an id and a source: %+v", bad)
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

// TestProbeReadsHerdrsHooksAndPlugins runs the two pipelines of the probe against a herdr stand-in, so the sed in them is tried on this machine's sed.
func TestProbeReadsHerdrsHooksAndPlugins(t *testing.T) {
	bin := t.TempDir()
	stub := "#!/bin/sh\ncase \"$1 $2\" in\n" +
		"\"integration status\") printf 'pi: not installed (/x)\\nclaude: outdated (v10 < v11) (/Users/x/.claude/hooks/herdr-agent-state.sh)\\ncodex: current (v10) (/y)\\nantigravity-cli: outdated (v2 < v3) (/z)\\nletta (experimental): outdated (v1 < v2) (/w)\\n' ;;\n" +
		"\"plugin list\") printf '2 plugins installed:\\n- herdr.push (Herdr Push) enabled [github:dcolinmorgan/herdr-push@f4fdb06d5413]\\n  config: /c\\n- local.one (Local) enabled [linked:/p]\\n' ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(pipeline string) string {
		cmd := exec.Command("sh", "-c", pipeline)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if got := run(OutdatedHooks); got != "claude v10 < v11\nantigravity-cli v2 < v3\nletta v1 < v2\n" {
		t.Fatalf("outdated hooks: %q", got)
	}
	if got := run(GitHubPlugins); got != "herdr.push dcolinmorgan/herdr-push\n" {
		t.Fatalf("github plugins: %q", got)
	}
	if probe := Script(); !strings.Contains(probe, OutdatedHooks+" | sed 's/^/hook=/'") || !strings.Contains(probe, GitHubPlugins+" | sed 's/^/plugin=/'") {
		t.Fatal("the probe does not print them as hook= and plugin= lines")
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
	if installedVia("../Cellar/claude-code/2.1.285/bin/claude") != "brew" || installedVia("/opt/homebrew/bin/claude") != "brew" || installedVia("../lib/node_modules/@anthropic-ai/claude-code/cli.js") != "npm" || installedVia("/Users/x/.local/bin/claude") != "native" || installedVia("/Users/x/.nvm/versions/node/v22/bin/claude") != "npm" || installedVia("/Users/x/.kimi-code/bin/kimi") != "native" || installedVia("/usr/bin/claude") != "" {
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

func TestParseReadsCodeServer(t *testing.T) {
	r, _ := Parse("os=macOS 26\ncode_server=4.140.0\n")
	if r.CodeServer != "4.140.0" {
		t.Fatalf("code_server = %q", r.CodeServer)
	}
	r, _ = Parse("os=macOS 26\n")
	if r.CodeServer != "" {
		t.Fatalf("code_server without the link = %q", r.CodeServer)
	}
}
