package review

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	if !scan.Incomplete {
		t.Error("a check that left files unread reads as complete")
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
	if err != nil || len(inner.Findings) != 2 || inner.Findings[0].File != "inner.txt" || inner.Findings[1].File != "staged.txt" || inner.Findings[1].Commit != secrets.Staged {
		t.Errorf("a folder inside the checkout reads its own work, and what is staged outside it, which its commit carries: %+v %v", inner.Findings, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if status() != before || string(after) != string(index) {
		t.Error("the check changed the checkout or its index")
	}
}

// TestAConflictInTheIndexIsNoFailedCheck: git names the unmerged file there, and its copy in the work is read.
func TestAConflictInTheIndexIsNoFailedCheck(t *testing.T) {
	answer := script.RS + "staged\n* Unmerged path app.go\n" + script.RS + "end\n"
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return answer, nil }}
	if _, err := ScanWork(host, "/repo", false); err != nil {
		t.Fatal(err)
	}
}

// TestTheCheckLeavesTheIndexAlone: a touched file's stat is not refreshed by a look at it.
func TestTheCheckLeavesTheIndexAlone(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("app.go", "package app\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "app.go"), later, later); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if _, err := ScanWork(shell.Local{Name: "local"}, dir, true); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, ".git", "index")); string(after) != string(before) {
		t.Error("the check rewrote the index")
	}
}

// TestTheCheckReadsThroughGitSettings: settings that change how a patch looks left the parser with nothing it knew, and
// a check that found nothing.
func TestTheCheckReadsThroughGitSettings(t *testing.T) {
	ownGitConfig(t)
	for name, settings := range map[string][][2]string{
		"colour":       {{"color.diff", "always"}, {"color.ui", "always"}},
		"no prefix":    {{"diff.noprefix", "true"}},
		"mnemonic":     {{"diff.mnemonicPrefix", "true"}},
		"own prefixes": {{"diff.srcPrefix", "x"}, {"diff.dstPrefix", "y"}},
		"relative":     {{"diff.relative", "true"}},
		"no root":      {{"log.showRoot", "false"}},
		"signature":    {{"log.showSignature", "true"}},
		"submodules":   {{"diff.submodule", "log"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir, git, write := scratchCheckout(t)
			_ = os.Mkdir(filepath.Join(dir, "sub"), 0o755)
			write("sub/first.sh", "export NPM_TOKEN="+"npm_"+fakeKey(21, 36)+"\n")
			git("add", "-A")
			git("commit", "-qm", "the first commit holds a token")
			write("sub/work.env", "DB_PASSWORD="+fakeKey(22, 24)+"\n")
			write("top.env", "API_SECRET="+fakeKey(24, 24)+"\n")
			git("add", "sub/work.env", "top.env")
			git("update-index", "--add", "--cacheinfo", "160000,"+git("rev-parse", "HEAD")+",vendored")
			write("sub/first.sh", "export NPM_TOKEN="+"npm_"+fakeKey(21, 36)+"\nDB_PASSWORD="+fakeKey(23, 24)+"\n")
			for _, setting := range settings {
				git("config", setting[0], setting[1])
			}
			scan, err := ScanWork(shell.Local{Name: "local"}, filepath.Join(dir, "sub"), true)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, finding := range scan.Findings {
				where := "work"
				if finding.Commit != "" && finding.Commit != secrets.Staged {
					where = "commit"
				}
				found[where+" "+finding.File+" "+finding.Rule] = true
			}
			for _, want := range []string{"commit sub/first.sh " + secrets.RuleNPM, "work first.sh " + secrets.RuleAssigned, "work top.env " + secrets.RuleAssigned} {
				if !found[want] {
					t.Errorf("no %q in %+v", want, scan.Findings)
				}
			}
			if len(scan.Findings) < 3 || scan.Commits != 1 {
				t.Errorf("%d commits, findings %+v", scan.Commits, scan.Findings)
			}
		})
	}
}

// TestAStreamThatFailedOrStoppedIsNoCleanCheck: a folder git cannot read, a git that failed half way and a script cut
// off all answered "nothing found" once, and the approval went out with its commit unchecked.
func TestAStreamThatFailedOrStoppedIsNoCleanCheck(t *testing.T) {
	for answer, want := range map[string]string{
		"":                                "it stopped before it had read everything",
		script.RS + "failed repository\n": "the folder is not a git checkout",
		script.RS + "failed folder\n":     "the folder is not there",
		"diff --git a/x b/x\n+++ b/x\n@@ -0,0 +1 @@\n+one\n":                          "it stopped before it had read everything",
		script.RS + "failed work\n" + script.RS + "end\n":                             "git could not read the changes",
		script.RS + "failed commits\n" + script.RS + "end\n":                          "git could not read the commits not pushed yet",
		"\x1b[1mdiff --git a/x b/x\x1b[m\n\x1b[32m+one\x1b[m\n" + script.RS + "end\n": "git's answer could not be read",
		"Submodule vendored abc..def:\n" + script.RS + "end\n":                        "git's answer could not be read",
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

// A folder inside the repository commits the whole index: what is staged outside it goes too, so it is read too.
func TestWhatIsStagedOutsideTheFolderIsReadToo(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	for _, folder := range []string{"web", "server"} {
		if err := os.MkdirAll(filepath.Join(dir, folder), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("web/app.txt", "hi\n")
	write("server/main.txt", "hi\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("web/web.env", "WEB_TOKEN="+fakeKey(80, 30)+"\n")
	write("server/server.env", "SERVER_TOKEN="+fakeKey(81, 30)+"\n")
	write("root.env", "ROOT_SECRET="+fakeKey(82, 30)+"\n")
	git("add", "web/web.env", "server/server.env", "root.env")

	scan, err := ScanWork(shell.Local{Name: "local"}, filepath.Join(dir, "web"), false)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]secrets.Finding{}
	for _, finding := range scan.Findings {
		found[finding.File] = finding
	}
	if len(scan.Findings) != 3 || found["web.env"].Commit != "" || found["server/server.env"].Commit != secrets.Staged || found["root.env"].Commit != secrets.Staged {
		t.Errorf("findings: %+v", scan.Findings)
	}
	top, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil || len(top.Findings) != 3 {
		t.Errorf("from the top, each once: %+v, %v", top.Findings, err)
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

// TestRemovedLinesCostTheCheckNothing: a big deletion leaves the lines added beside it read.
func TestRemovedLinesCostTheCheckNothing(t *testing.T) {
	ownGitConfig(t)
	_, git, write := scratchCheckout(t)
	write("data.txt", strings.Repeat("an old line of data\n", scanPerFile/10))
	git("add", "-A")
	git("commit", "-qm", "init")
	write("data.txt", "export NPM_TOKEN="+"npm_"+fakeKey(21, 36)+"\n")
	dir := strings.TrimSpace(git("rev-parse", "--show-toplevel"))
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 || scan.Incomplete || len(scan.Skipped) != 0 {
		t.Errorf("scan: %+v", scan)
	}
}

func TestABinaryFileAloneLeavesTheCheckComplete(t *testing.T) {
	scan, err := readScan("diff --git a/logo.png b/logo.png\nBinary files /dev/null and b/logo.png differ\n"+script.RS+"end\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Incomplete || len(scan.Skipped) != 1 {
		t.Errorf("scan: %+v", scan)
	}
}

func TestABigUntrackedFileIsCalledBinaryOnlyWhenItIs(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("shot.png", "\x00"+strings.Repeat("x", scanPerFile+1))
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Incomplete || len(scan.Skipped) != 1 || scan.Skipped[0] != (Skipped{Path: "shot.png", Why: SkipBinary}) {
		t.Errorf("a big binary: %+v", scan)
	}
	write("dump.sql", strings.Repeat("insert\n", scanPerFile/7+1))
	if scan, err = ScanWork(shell.Local{Name: "local"}, dir, false); err != nil {
		t.Fatal(err)
	}
	if !scan.Incomplete {
		t.Errorf("big text left unread reads as complete: %+v", scan)
	}
}

// TestAKeystorePastTheBudgetIsStillFound, and what a push would send past it counts against the push.
func TestAKeystorePastTheBudgetIsStillFound(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	for i := range scanTotal/scanPerFile + 2 {
		write("part"+strconv.Itoa(i+10)+".txt", strings.Repeat("a line\n", scanPerFile/7-1))
	}
	write("z.txt", "after the budget\n")
	write("z.p12", "\x00\x01 a keystore")
	git("add", "-A")
	git("commit", "-qm", "a lot")
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range scan.Findings {
		found = found || finding.File == "z.p12"
	}
	if !found {
		t.Errorf("the keystore went unnoticed: %+v", scan.Findings)
	}
	if !scan.Incomplete || !scan.PushUnread() {
		t.Errorf("text past the budget reads as read: %+v", scan.Skipped)
	}
}

// TestWorkBesideTheCommitsLeavesThemTheirBudget: what a push does not send cannot leave what it sends unread.
func TestWorkBesideTheCommitsLeavesThemTheirBudget(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("sent.txt", "export NPM_TOKEN="+"npm_"+fakeKey(22, 36)+"\n")
	git("add", "-A")
	git("commit", "-qm", "the commit a push sends")
	for i := range scanTotal/scanPerFile + 2 {
		write("scratch"+strconv.Itoa(i+10)+".txt", strings.Repeat("a line\n", scanPerFile/7-1))
	}
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.Incomplete || scan.PushUnread() {
		t.Errorf("skipped: %+v", scan.Skipped)
	}
	found := false
	for _, finding := range scan.Findings {
		found = found || finding.File == "sent.txt"
	}
	if !found {
		t.Errorf("the commit went unread: %+v", scan.Findings)
	}
}

// conflictedMerge is a checkout whose unpushed merge resolved a conflict with a line only the merge holds.
func conflictedMerge(t *testing.T, resolution string) string {
	t.Helper()
	dir, git, write := scratchCheckout(t)
	write("a.txt", "one\ntwo\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	git("checkout", "-qb", "side")
	write("a.txt", "one\nside\n")
	git("commit", "-qam", "side")
	git("checkout", "-q", "main")
	write("a.txt", "one\nmain\n")
	git("commit", "-qam", "main")
	cmd := exec.Command("git", "-c", "user.email=t@t", "-c", "user.name=t", "merge", "-q", "side")
	cmd.Dir = dir
	_ = cmd.Run() // it stops at the conflict
	write("a.txt", "one\n"+resolution+"\n")
	git("add", "-A")
	git("commit", "-qm", "merged")
	return dir
}

func TestASecretOnlyAMergeCommitHoldsIsFound(t *testing.T) {
	ownGitConfig(t)
	secret := "export NPM_TOKEN=" + "npm_" + fakeKey(23, 36)
	scan, err := ScanWork(shell.Local{Name: "local"}, conflictedMerge(t, secret), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 || scan.Findings[0].File != "a.txt" || scan.Findings[0].Line != 2 || scan.Findings[0].Commit == "" {
		t.Errorf("the conflict's resolution went unread: %+v", scan.Findings)
	}
}

func TestAMergeThatOnlyMergedHoldsNoPush(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("a.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	git("checkout", "-qb", "side")
	write("a.txt", "one\n2\n3\n4\n5\n6\n7\n8\n9\n")
	git("commit", "-qam", "side")
	git("checkout", "-q", "main")
	write("a.txt", "1\n2\n3\n4\n5\n6\n7\n8\nnine\n")
	git("commit", "-qam", "main")
	git("merge", "-q", "--no-edit", "side")
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Incomplete || scan.PushUnread() || len(scan.Findings) != 0 {
		t.Errorf("%+v", scan)
	}
}

func TestALineAddedByAMergeOfSeveralBranchesIsRead(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	for _, branch := range []string{"one", "two"} {
		git("checkout", "-qb", branch, "main")
		write(branch+".txt", branch+"\n")
		git("add", "-A")
		git("commit", "-qm", branch)
	}
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-commit", "one", "two")
	write("a.txt", "a\nexport NPM_TOKEN="+"npm_"+fakeKey(24, 36)+"\n")
	git("add", "-A")
	git("commit", "-qm", "merged")
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 || scan.Findings[0].File != "a.txt" || scan.Findings[0].Line != 2 {
		t.Errorf("%+v", scan.Findings)
	}
}

// mergeBringing is a checkout whose unpushed merge, of branches that touch other files, adds one file of its own.
func mergeBringing(t *testing.T, name, body string) string {
	t.Helper()
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	git("checkout", "-qb", "side")
	write("b.txt", "b\n")
	git("add", "-A")
	git("commit", "-qm", "side")
	git("checkout", "-q", "main")
	write("c.txt", "c\n")
	git("add", "-A")
	git("commit", "-qm", "main")
	git("merge", "-q", "--no-commit", "side")
	write(name, body)
	git("add", "-A")
	git("commit", "-qm", "merged")
	return dir
}

func TestAKeystoreOnlyAMergeBringsIsFound(t *testing.T) {
	ownGitConfig(t)
	scan, err := ScanWork(shell.Local{Name: "local"}, mergeBringing(t, "store.p12", "\x00\x01binary\x00"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 || scan.Findings[0].File != "store.p12" || scan.PushUnread() {
		t.Errorf("%+v", scan)
	}
}

func TestABinaryFileOfAMergeStaysUnread(t *testing.T) {
	ownGitConfig(t)
	scan, err := ScanWork(shell.Local{Name: "local"}, mergeBringing(t, "blob.bin", "\x00\x01binary\x00"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.PushUnread() || !scan.Incomplete || len(scan.Skipped) != 1 || scan.Skipped[0].Path != "blob.bin" || scan.Skipped[0].Why != SkipBinary {
		t.Errorf("a merge's binary file counted as read: %+v", scan)
	}
}

func TestATextFileAMergeBringsIsReadThoughMarkedBinary(t *testing.T) {
	ownGitConfig(t)
	dir := mergeBringing(t, "brought.env", "export NPM_TOKEN="+"npm_"+fakeKey(34, 36)+"\n")
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.env -diff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !gitHasAttrSource(t) {
		t.Skip("git before 2.41")
	}
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 || scan.Findings[0].File != "brought.env" {
		t.Errorf("%+v", scan.Findings)
	}
}

func TestAMergeReadThatBrokeOffIsNoCleanCheck(t *testing.T) {
	ownGitConfig(t)
	dir := mergeBringing(t, "d.txt", "d\n")
	real, err := exec.LookPath("awk")
	if err != nil {
		t.Skip("no awk")
	}
	bin := t.TempDir()
	stand := "#!/bin/sh\ncase \"$*\" in *'diff --cc'*) cat >/dev/null; exit 2;; esac\nexec " + script.Quote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "awk"), []byte(stand), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if scan, err := ScanWork(shell.Local{Name: "local"}, dir, true); err == nil {
		t.Errorf("a check that lost the merges counted as finished: %+v", scan)
	}
}

func TestAKeystoreAMergeDeletedIsNoFinding(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("store.p12", "\x00\x01binary\x00")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	git("checkout", "-qb", "side")
	write("b.txt", "b\n")
	git("add", "-A")
	git("commit", "-qm", "side")
	git("checkout", "-q", "main")
	write("c.txt", "c\n")
	git("add", "-A")
	git("commit", "-qm", "main")
	git("update-ref", "refs/remotes/origin/main", "main")
	git("update-ref", "refs/remotes/origin/side", "side")
	git("merge", "-q", "--no-commit", "side")
	git("rm", "-q", "store.p12")
	git("commit", "-qm", "merged")
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 0 || scan.PushUnread() {
		t.Errorf("%+v", scan)
	}
}

func TestAPartialCloneOnAnOlderGitLeavesItsMergesUnread(t *testing.T) {
	ownGitConfig(t)
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git")
	}
	bin := t.TempDir()
	stand := "#!/bin/sh\n[ \"$1\" = --no-lazy-fetch ] && exit 129\nexec " + script.Quote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(stand), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for key, value := range map[string]string{"remote.origin.promisor": "true", "remote.origin.partialCloneFilter": "blob:none", "extensions.partialClone": "origin"} {
		dir := conflictedMerge(t, "export NPM_TOKEN="+"npm_"+fakeKey(25, 36))
		config := exec.Command(real, "config", key, value)
		config.Dir = dir
		if err := config.Run(); err != nil {
			t.Fatal(err)
		}
		scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if !scan.PushUnread() || !scan.Incomplete || len(scan.Findings) != 0 {
			t.Errorf("%s: merges left unread must say so: %+v", key, scan)
		}
	}
}

// gitHasAttrSource says this git can be told to ignore the repository's attributes.
func gitHasAttrSource(t *testing.T) bool {
	t.Helper()
	cmd := exec.Command("git", "--attr-source=4b825dc642cb6eb9a060e54bf8d69288fbee4904", "--version")
	return cmd.Run() == nil
}

// markedCheckout has a committed, a staged, a changed and a new file marked binary that hold a secret each, and real
// binary files beside them.
func markedCheckout(t *testing.T) string {
	t.Helper()
	secret := func(seed uint32) string { return "export NPM_TOKEN=" + "npm_" + fakeKey(seed, 36) + "\n" }
	dir, git, write := scratchCheckout(t)
	write(".gitattributes", "*.env -diff\n*.bin binary\n")
	write("work.env", "A=1\n")
	write("staged.env", "A=1\n")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("sub/keep.txt", "k\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	write("sent.env", secret(31))
	write("sent.bin", "\x00\x01binary\x00")
	git("add", "-A")
	git("commit", "-qm", "the commit a push sends")
	write("work.env", "A=1\n"+secret(32))
	write("staged.env", "A=1\n"+secret(33))
	write("new.env", secret(35))
	write("real.bin", "\x00\x01binary\x00")
	write("new.bin", "\x00\x01binary\x00")
	git("add", "staged.env", "real.bin")
	return dir
}

func TestAFileMarkedBinaryIsReadAsTheTextItIs(t *testing.T) {
	ownGitConfig(t)
	if !gitHasAttrSource(t) {
		t.Skip("git before 2.41")
	}
	dir := markedCheckout(t)
	for _, from := range []string{dir, filepath.Join(dir, "sub")} {
		scan, err := ScanWork(shell.Local{Name: "local"}, from, true)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, finding := range scan.Findings {
			found[filepath.Base(finding.File)] = true
		}
		want := []string{"sent.env", "staged.env"}
		if from == dir { // the work and the new files are the folder's
			want = append(want, "work.env", "new.env")
		}
		for _, file := range want {
			if !found[file] {
				t.Errorf("from %s, %s went unread: %+v", from, file, scan.Findings)
			}
		}
		if scan.PushUnread() {
			t.Errorf("a real binary file of a commit holds the push: %+v", scan.Skipped)
		}
	}
}

// gitWithoutAttrSource puts a git first on PATH that refuses --attr-source as one before 2.41 does.
func gitWithoutAttrSource(t *testing.T) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git")
	}
	bin := t.TempDir()
	stand := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --attr-source=*) exit 129 ;; esac; done\nexec " + script.Quote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(stand), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAnOlderGitLeavesWhatAttributesMayHideUnread(t *testing.T) {
	ownGitConfig(t)
	dir := markedCheckout(t)
	gitWithoutAttrSource(t)
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.PushUnread() || !scan.Incomplete {
		t.Errorf("a file an attribute may be hiding counted as read: %+v", scan)
	}
	named := false
	for _, skip := range scan.Skipped {
		named = named || (skip.Why == SkipMarked && skip.Path == "sent.env" && skip.InCommit)
	}
	if !named {
		t.Errorf("the file is not named: %+v", scan.Skipped)
	}
}

func TestACommentInAnAttributesFileMarksNothing(t *testing.T) {
	ownGitConfig(t)
	dir := mergeBringing(t, ".gitattributes", "# binary files are stored as they are\n*.txt text\n")
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte("\x00\x01binary\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitWithoutAttrSource(t)
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if scan.PushUnread() || scan.Incomplete {
		t.Errorf("%+v", scan)
	}
}

func TestAnOlderGitTrustsARepositoryThatMarksNothing(t *testing.T) {
	ownGitConfig(t)
	dir := mergeBringing(t, "plain.txt", "p\n")
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte("\x00\x01binary\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitWithoutAttrSource(t)
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if scan.PushUnread() || scan.Incomplete {
		t.Errorf("%+v", scan)
	}
}

func TestLocalAttributesLeaveWhatTheyMayHideUnread(t *testing.T) {
	ownGitConfig(t)
	dir := mergeBringing(t, "plain.txt", "p\n")
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "attributes"), []byte("*.env -diff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "local.env"), []byte("\x00not text\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.Incomplete {
		t.Errorf("%+v", scan)
	}
}

func TestAnEncodedWorkingTreeFileIsRead(t *testing.T) {
	ownGitConfig(t)
	utf16 := func(s string) string {
		var b strings.Builder
		for i := range len(s) {
			b.WriteByte(s[i])
			b.WriteByte(0)
		}
		return b.String()
	}
	dir, git, write := scratchCheckout(t)
	write(".gitattributes", "*.ps1 text working-tree-encoding=UTF-16LE\n")
	write("run.ps1", utf16("A=1\n"))
	git("add", "-A")
	git("commit", "-qm", "init")
	write("run.ps1", utf16("A=1\nexport NPM_TOKEN="+"npm_"+fakeKey(36, 36)+"\n"))
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 || scan.Findings[0].File != "run.ps1" {
		t.Errorf("%+v", scan)
	}
}

func TestADiffDriverCalledBinaryLeavesItsFilesUnread(t *testing.T) {
	ownGitConfig(t)
	dir := mergeBringing(t, "hidden.env", "export NPM_TOKEN="+"npm_"+fakeKey(37, 36)+"\n")
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "attributes"), []byte("*.env diff=opaque\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := exec.Command("git", "config", "diff.opaque.binary", "true")
	config.Dir = dir
	if err := config.Run(); err != nil {
		t.Fatal(err)
	}
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.PushUnread() || !scan.Incomplete {
		t.Errorf("%+v", scan)
	}
}

func TestAnOlderGitHeedsAnUntrackedAttributesFile(t *testing.T) {
	ownGitConfig(t)
	dir := mergeBringing(t, "hidden.env", "export NPM_TOKEN="+"npm_"+fakeKey(38, 36)+"\n")
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.env -diff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitWithoutAttrSource(t)
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.PushUnread() || !scan.Incomplete {
		t.Errorf("%+v", scan)
	}
}

func TestAWorkFileReadTheSecondTimeIsNotSaidUnread(t *testing.T) {
	ownGitConfig(t)
	if !gitHasAttrSource(t) {
		t.Skip("git before 2.41")
	}
	dir, git, write := scratchCheckout(t)
	write(".gitattributes", "*.env -diff\n")
	write("producción \"quoted\".env", "A=1\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("producción \"quoted\".env", "A=1\nexport NPM_TOKEN="+"npm_"+fakeKey(39, 36)+"\n")
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 || len(scan.Skipped) != 0 {
		t.Errorf("%+v", scan)
	}
}

func TestAnEncodedFileMarkedBinaryIsUnread(t *testing.T) {
	ownGitConfig(t)
	if !gitHasAttrSource(t) {
		t.Skip("git before 2.41")
	}
	utf16 := func(s string) string {
		var b strings.Builder
		for i := range len(s) {
			b.WriteByte(s[i])
			b.WriteByte(0)
		}
		return b.String()
	}
	dir, git, write := scratchCheckout(t)
	write(".gitattributes", "*.ps1 text working-tree-encoding=UTF-16LE -diff\n")
	write("run.ps1", utf16("A=1\n"))
	git("add", "-A")
	git("commit", "-qm", "init")
	write("run.ps1", utf16("A=1\nB=2\n"))
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.Incomplete || len(scan.Skipped) != 1 || scan.Skipped[0] != (Skipped{Path: "run.ps1", Why: SkipMarked}) {
		t.Errorf("%+v", scan)
	}
}

func TestTheDefaultDiffDriverCalledBinaryHidesNothing(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("config", "diff.default.binary", "true")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	write("app.go", "const token = \""+"npm_"+fakeKey(40, 36)+"\"\n")
	git("add", "-A")
	git("commit", "-qm", "the commit a push sends")
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) != 1 {
		t.Errorf("%+v", scan)
	}
}

func TestAnOlderGitHeedsTheIndexedAttributes(t *testing.T) {
	ownGitConfig(t)
	dir, git, write := scratchCheckout(t)
	write(".gitattributes", "*.env -diff\n")
	write("app.env", "A=1\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("app.env", "A=1\nexport NPM_TOKEN="+"npm_"+fakeKey(41, 36)+"\n")
	git("add", "app.env")
	if err := os.Remove(filepath.Join(dir, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	gitWithoutAttrSource(t)
	scan, err := ScanWork(shell.Local{Name: "local"}, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !scan.Incomplete {
		t.Errorf("%+v", scan)
	}
}
