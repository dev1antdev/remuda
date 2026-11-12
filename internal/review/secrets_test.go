package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/secrets"
	"github.com/dev1antdev/remuda/internal/shell"
)

// fakeKey is n bytes that look as random as a key, made when the test runs: no secret sits in this file.
func fakeKey(seed uint32, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	state := seed*2654435761 + 12345
	out := make([]byte, n)
	for i := range out {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		out[i] = alphabet[state%uint32(len(alphabet))]
	}
	return string(out)
}

// ownGitConfig keeps the person's global git settings out of a test: their excludes file ignores
// .claude/settings.local.json, as Claude Code sets it up, and would decide what these tests check.
func ownGitConfig(t *testing.T) {
	t.Helper()
	empty := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", empty)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(empty, "gitconfig"))
}

// TestTheSecretCheckReadsTheScriptsAnswer pins the stream secretScript prints, with a recorded answer and no git.
func TestTheSecretCheckReadsTheScriptsAnswer(t *testing.T) {
	answer := strings.Join([]string{
		"diff --git a/config/app.yml b/config/app.yml",
		"index 1111111..2222222 100644",
		"--- a/config/app.yml",
		"+++ b/config/app.yml",
		"@@ -10,0 +11,2 @@",
		"+name: app",
		"+api_key: " + fakeKey(1, 32),
		"diff --git a/src/server.go b/src/server.go",
		"--- a/src/server.go",
		"+++ b/src/server.go",
		"@@ -3 +3 @@",
		"-const ghToken = os.Getenv(\"GH\")",
		"+const ghToken = \"" + "ghp_" + fakeKey(2, 36) + "\"",
		"@@ -20,2 +20,0 @@",
		"-gone",
		"-gone too",
		script.RS + "many 7",
		"diff --git a/creds/store.p12 b/creds/store.p12",
		"new file mode 100644",
		"index 0000000..3333333",
		"Binary files /dev/null and b/creds/store.p12 differ",
		"diff --git a/logo.png b/logo.png",
		"Binary files /dev/null and b/logo.png differ",
		"diff --git a/art/cats and dogs.jks b/art/cats and dogs.jks",
		"Binary files a/art/cats and dogs.jks and b/art/cats and dogs.jks differ",
		"diff --git a/conf/with space.env b/conf/with space.env",
		"--- a/conf/with space.env",
		"+++ b/conf/with space.env\t",
		"@@ -1,0 +2 @@",
		"+SECRET_KEY=" + fakeKey(6, 40),
		script.RS + "large dump.sql",
		`diff --git "a/notes/new \303\251.md" "b/notes/new \303\251.md"`,
		"new file mode 100644",
		"--- /dev/null",
		`+++ "b/notes/new \303\251.md"`,
		"@@ -0,0 +1,3 @@",
		"+# Notes",
		"++++ a line that starts with three pluses",
		"+token: " + "glpat-" + fakeKey(3, 20),
		"diff --git a/huge.txt b/huge.txt",
		"--- /dev/null",
		"+++ b/huge.txt",
		"@@ -0,0 +1,100 @@",
		"+fine",
		script.RS + "partly",
		script.RS + "budget b/after.txt",
		script.RS + "untracked",
		script.RS + "file b/q\"uote.env",
		"+NAME=app",
		"+DB_PASSWORD=" + fakeKey(7, 24),
		script.RS + "binary keys/new.p12",
		script.RS + "binary photo.jpg",
		script.RS + "unread later.txt",
		script.RS + "staged",
		"diff --git a/src/server.go b/src/server.go",
		"+++ b/src/server.go",
		"@@ -3 +3 @@",
		"+const ghToken = \"" + "ghp_" + fakeKey(2, 36) + "\"",
		"diff --git a/logo.png b/logo.png",
		"Binary files /dev/null and b/logo.png differ",
		"diff --git a/.env b/.env",
		"+++ b/.env",
		"@@ -0,0 +1 @@",
		"+STRIPE=" + "sk_live_" + fakeKey(8, 24),
		script.RS + "older 5",
		script.RS + "commit abc1234",
		"",
		"diff --git a/deploy.sh b/deploy.sh",
		"--- a/deploy.sh",
		"+++ b/deploy.sh",
		"@@ -4,0 +5 @@",
		"+export NPM_TOKEN=" + "npm_" + fakeKey(4, 36),
		"diff --git a/old.sh b/old.sh",
		"deleted file mode 100644",
		"--- a/old.sh",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-export GH=" + "ghp_" + fakeKey(5, 36),
		script.RS + "end",
		"",
	}, "\n")
	host := &shell.Fake{Name: "box", Remote: true, Answer: func(string) (string, error) { return answer, nil }}

	scan, err := ScanWork(host, "/repo", true)
	if err != nil {
		t.Fatal(err)
	}
	type place struct {
		file, rule, commit string
		line               int
	}
	var got []place
	for _, finding := range scan.Findings {
		got = append(got, place{finding.File, finding.Rule, finding.Commit, finding.Line})
	}
	want := []place{
		{"config/app.yml", secrets.RuleAssigned, "", 12},
		{"src/server.go", secrets.RuleGitHub, "", 3},
		{"creds/store.p12", secrets.RuleKeyFile, "", 0},
		{"art/cats and dogs.jks", secrets.RuleKeyFile, "", 0},
		{"conf/with space.env", secrets.RuleAssigned, "", 2},
		{"notes/new é.md", secrets.RuleGitLab, "", 3},
		{"b/q\"uote.env", secrets.RuleAssigned, "", 2},
		{"keys/new.p12", secrets.RuleKeyFile, "", 0},
		{".env", secrets.RuleStripe, secrets.Staged, 1},
		{"deploy.sh", secrets.RuleNPM, "abc1234", 5},
	}
	if len(got) != len(want) {
		t.Fatalf("findings: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("finding %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	skipped, counted := map[string]string{}, map[string]int{}
	for _, skip := range scan.Skipped {
		skipped[skip.Path] = skip.Why
		if skip.Path == "" {
			counted[skip.Why] = skip.Count
		}
	}
	if len(scan.Skipped) != 8 || skipped["creds/store.p12"] != "" || skipped["logo.png"] != SkipBinary || skipped["dump.sql"] != SkipLarge ||
		skipped["huge.txt"] != SkipPartly || skipped["after.txt"] != SkipBudget || skipped["photo.jpg"] != SkipBinary || skipped["later.txt"] != SkipBudget ||
		counted[SkipMany] != 7 || counted[SkipOlder] != 5 {
		t.Errorf("skipped: %+v", scan.Skipped)
	}
	if scan.Files != 6 || scan.Commits != 1 || !scan.Push {
		t.Errorf("read %d files, %d commits, push %v", scan.Files, scan.Commits, scan.Push)
	}
	scripts := host.Scripts()
	if len(scripts) != 1 || !strings.Contains(scripts[0], "cd '/repo'") || !strings.Contains(scripts[0], "--not --remotes") || !strings.Contains(scripts[0], "--no-textconv") {
		t.Fatalf("script: %q", scripts)
	}
	if _, err := ScanWork(host, "/repo", false); err != nil || strings.Contains(host.Scripts()[1], "--not --remotes") {
		t.Errorf("a commit's check reads no commits: %v", err)
	}
}

func TestTheSecretCheckOnARealCheckoutChangesNothing(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("app.env", "NAME=app\n")
	write("pushed.sh", "echo hi\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("pushed.sh", "echo hi\nexport NPM_TOKEN="+"npm_"+fakeKey(10, 36)+"\n")
	git("commit", "-qam", "a token in a commit")
	write("app.env", "NAME=app\nDB_PASSWORD="+fakeKey(11, 24)+"\n")
	write(".gitignore", "ignored.env\n")
	write("ignored.env", "TOKEN="+"ghp_"+fakeKey(12, 36)+"\n")
	write("staged.txt", "one\nmaps: "+"AIza"+fakeKey(13, 35)+"\n")
	git("add", "staged.txt")
	write("big.log", strings.Repeat("a line of a log\n", scanPerFile/16+1))
	write("store.p12", "\x00\x01\x02 a keystore")
	_ = os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	write("sub/inner.txt", "client_secret: "+fakeKey(14, 30)+"\n")
	status := func() string { return git("status", "--porcelain=v1", "-uall") }
	index, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	before := status()

	machine := shell.Local{Name: "local"}
	scan, err := ScanWork(machine, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]int{}
	for _, finding := range scan.Findings {
		found[finding.File+" "+finding.Rule] = finding.Line
		if finding.Commit != "" {
			t.Errorf("a commit's check reads no commit: %+v", finding)
		}
	}
	if len(found) != 4 || found["app.env "+secrets.RuleAssigned] != 2 || found["staged.txt "+secrets.RuleGoogle] != 2 ||
		found["sub/inner.txt "+secrets.RuleAssigned] != 1 || found["store.p12 "+secrets.RuleKeyFile] != 0 {
		t.Errorf("findings: %+v", scan.Findings)
	}
	why := map[string]string{}
	for _, skip := range scan.Skipped {
		why[skip.Path] = skip.Why
	}
	if why["big.log"] != SkipLarge || why["store.p12"] != "" {
		t.Errorf("skipped: %+v", scan.Skipped)
	}

	pushed, err := ScanWork(machine, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	var fromCommit *secrets.Finding
	for i, finding := range pushed.Findings {
		if finding.Commit != "" {
			fromCommit = &pushed.Findings[i]
		}
	}
	if fromCommit == nil || fromCommit.File != "pushed.sh" || fromCommit.Line != 2 || fromCommit.Rule != secrets.RuleNPM || pushed.Commits != 2 {
		t.Errorf("a push's check reads the commits on no remote: %+v, %d commits", pushed.Findings, pushed.Commits)
	}

	inner, err := ScanWork(machine, filepath.Join(dir, "sub"), false)
	if err != nil || len(inner.Findings) != 1 || inner.Findings[0].File != "inner.txt" {
		t.Errorf("a folder inside the checkout reads its own work: %+v %v", inner.Findings, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if status() != before || string(after) != string(index) {
		t.Error("the check changed the checkout or its index")
	}
}

// TestAStreamThatFailedOrStoppedIsNoCleanCheck: a folder git cannot read, a git that failed half way and a script cut
// off all answered "nothing found" once, and the approval went out with its commit unchecked.
func TestAStreamThatFailedOrStoppedIsNoCleanCheck(t *testing.T) {
	for answer, want := range map[string]string{
		"":                                "it stopped before it had read everything",
		script.RS + "failed repository\n": "the folder is not a git checkout",
		script.RS + "failed folder\n":     "the folder is not there",
		"diff --git a/x b/x\n+++ b/x\n@@ -0,0 +1 @@\n+one\n": "it stopped before it had read everything",
		script.RS + "failed work\n" + script.RS + "end\n":    "git could not read the changes",
		script.RS + "failed commits\n" + script.RS + "end\n": "git could not read the commits not pushed yet",
	} {
		host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return answer, nil }}
		if _, err := ScanWork(host, "/repo", true); err == nil || err.Error() != want {
			t.Errorf("%q: %v, want %q", answer, err, want)
		}
	}

	ownGitConfig(t)
	if _, err := ScanWork(shell.Local{Name: "local"}, t.TempDir(), false); err == nil || err.Error() != "the folder is not a git checkout" {
		t.Errorf("a folder outside git: %v", err)
	}
}

// TestTheCheckReadsEveryUntrackedFileByItsOwnName: a name with a blank, a quote, a tab or a leading dash is read, a
// link is not followed out of the checkout, and a push's commits are read whole, outside the folder too.
func TestTheCheckReadsEveryUntrackedFileByItsOwnName(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	_ = os.Mkdir(filepath.Join(dir, "app"), 0o755)
	write("app/main.go", "package main\n")
	write("README.md", "hi\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("shared.env", "DEPLOY_TOKEN="+fakeKey(40, 30)+"\n")
	git("add", "shared.env")
	git("commit", "-qm", "a token outside the app's folder")
	names := []string{"app/with space.env", `app/q"uote.env`, "app/tab\there.env", "app/-dash.env", "app/ünï.env"}
	for i, name := range names {
		write(name, "NAME=app\nSERVICE_PASSWORD="+fakeKey(uint32(41+i), 24)+"\n")
	}
	outside := filepath.Join(t.TempDir(), "elsewhere.env")
	_ = os.WriteFile(outside, []byte("TOKEN="+"ghp_"+fakeKey(50, 36)+"\n"), 0o644)
	_ = os.Symlink(outside, filepath.Join(dir, "app", "link.env"))

	scan, err := ScanWork(shell.Local{Name: "local"}, filepath.Join(dir, "app"), true)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, finding := range scan.Findings {
		found[finding.File] = finding.Commit
		if finding.Line != 2 && finding.Commit == "" || finding.Line != 1 && finding.Commit != "" {
			t.Errorf("the line: %+v", finding)
		}
	}
	for _, name := range names {
		if commit, ok := found[strings.TrimPrefix(name, "app/")]; !ok || commit != "" {
			t.Errorf("%q is not read: %+v", name, scan.Findings)
		}
	}
	if commit, ok := found["shared.env"]; !ok || commit == "" {
		t.Errorf("the commit's file outside the folder is not read: %+v", scan.Findings)
	}
	if _, ok := found["link.env"]; ok || len(scan.Findings) != len(names)+1 || scan.Files != len(names) {
		t.Errorf("a link is followed, or more is read: %d files, %+v", scan.Files, scan.Findings)
	}
}

// TestWhatIsStagedIsReadToo: git commit carries the index, and a secret staged then taken out of the file, or a staged
// file deleted since, went out unread when only the files were.
func TestWhatIsStagedIsReadToo(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("app.txt", "hi\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("deploy.env", "REGION=eu\nDEPLOY_TOKEN="+fakeKey(70, 30)+"\n")
	write("gone.env", "API_SECRET="+fakeKey(71, 30)+"\n")
	write("kept.env", "DB_PASSWORD="+fakeKey(72, 24)+"\n")
	git("add", "deploy.env", "gone.env", "kept.env")
	write("deploy.env", "REGION=eu\nDEPLOY_TOKEN=\n")
	_ = os.Remove(filepath.Join(dir, "gone.env"))

	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]secrets.Finding{}
	for _, finding := range scan.Findings {
		found[finding.File] = finding
	}
	if len(scan.Findings) != 3 || found["deploy.env"].Commit != secrets.Staged || found["deploy.env"].Line != 2 ||
		found["gone.env"].Commit != secrets.Staged || found["kept.env"].Commit != "" {
		t.Errorf("findings: %+v", scan.Findings)
	}
}

// TestManyUntrackedFilesAreReadInSeconds: one git process per untracked file took twenty seconds for two thousand
// on a Mac, more than the check's bound on a slower machine.
func TestManyUntrackedFilesAreReadInSeconds(t *testing.T) {
	ownGitConfig(t)
	dir, _, write := scratchCheckout(t)
	for i := range scanUntracked + 10 {
		write(fmt.Sprintf("f%04d.txt", i), "hello\n")
	}
	write("zz.env", "API_TOKEN="+fakeKey(60, 30)+"\n")
	start := time.Now()
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if took := time.Since(start); err != nil || took > 5*time.Second {
		t.Fatalf("%d untracked files took %s: %v", scanUntracked+11, took, err)
	}
	if scan.Files != scanUntracked || len(scan.Skipped) != 1 || scan.Skipped[0].Why != SkipMany || scan.Skipped[0].Count != 11 {
		t.Errorf("read %d files, skipped %+v", scan.Files, scan.Skipped)
	}
}

func TestTheLensAuditsClaudeCodesSettings(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	_ = os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "web"), 0o755)
	write(".claude/settings.local.json", `{"permissions": {"allow": [`+"\n"+`"Bash(npm test)",`+"\n"+
		`"Bash(curl -H 'Authorization: Bearer `+fakeKey(20, 40)+`' https://api.internal/deploy)"`+"\n]}}\n")
	write(".claude/settings.json", `{"permissions": {"allow": ["Bash(make test)"]}}`+"\n")
	write("web/app.ts", "export {}\n")
	git("add", "-A")
	git("commit", "-qm", "settings committed by mistake")

	store := New(t.TempDir())
	machine := shell.Local{Name: "local"}
	summary := store.Summary(machine, dir)
	if len(summary.Settings) != 2 {
		t.Fatalf("settings: %+v (%s)", summary.Settings, summary.Error)
	}
	local, shared := summary.Settings[0], summary.Settings[1]
	if local.Path != ".claude/settings.local.json" || !local.Local || !local.Tracked || local.Ignored || len(local.Findings) != 1 ||
		local.Findings[0].Line != 3 || local.Findings[0].Rule != secrets.RuleAuthHeader {
		t.Errorf("the local file: %+v", local)
	}
	if shared.Path != ".claude/settings.json" || shared.Local || !shared.Tracked || len(shared.Findings) != 0 {
		t.Errorf("the shared file: %+v", shared)
	}

	inner := store.Summary(machine, filepath.Join(dir, "web"))
	if len(inner.Settings) != 2 || !filepath.IsAbs(inner.Settings[0].Path) || !strings.HasSuffix(inner.Settings[0].Path, "/.claude/settings.local.json") {
		t.Errorf("a folder inside the repository hears of its root's settings: %+v", inner.Settings)
	}

	git("rm", "-q", "--cached", ".claude/settings.local.json")
	write(".gitignore", ".claude/settings.local.json\n")
	summary = store.Summary(machine, dir)
	if len(summary.Settings) != 2 || summary.Settings[0].Tracked || !summary.Settings[0].Ignored {
		t.Errorf("untracked and ignored: %+v", summary.Settings)
	}
}

func TestTheSettingsSectionIsRead(t *testing.T) {
	section := ".claude/settings.local.json" + script.US + "no" + script.US + "no\n" +
		`{"allow": ["Bash(GITHUB_TOKEN=` + "ghp_" + fakeKey(30, 36) + ` gh pr list)"]}` + "\n" + script.GS + "\n" +
		"/repo/.claude/settings.json" + script.US + "yes" + script.US + "no\n{}\n" + script.GS + "\n"
	audits := readSettings(section)
	if len(audits) != 2 || !audits[0].Local || audits[0].Tracked || audits[0].Ignored || len(audits[0].Findings) != 1 ||
		audits[1].Local || !audits[1].Tracked || audits[1].Path != "/repo/.claude/settings.json" {
		t.Errorf("%+v", audits)
	}
}
