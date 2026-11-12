package health

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// authStatus is `claude auth status --json` as Claude Code 2.1.322 prints it, the account made up
const authStatus = `{
  "loggedIn": true,
  "authMethod": "claude.ai",
  "apiProvider": "firstParty",
  "analyticsDisabled": false,
  "projectsDirectory": "/Users/someone/.claude/projects",
  "configDirectory": "/Users/someone/.claude",
  "loginExpiresAt": "2026-12-05T16:18:16.555Z",
  "email": "someone@example.com",
  "orgId": "00000000-0000-4000-8000-000000000000",
  "orgName": "someone@example.com's Organization, \"loginExpiresAt\": \"2030-01-01T00:00:00Z\"",
  "subscriptionType": "max",
  "storedClaudeAiLogin": true,
  "planTier": "max_20x"
}
`

// runWithClaude runs a script with a claude stand-in first on a PATH that holds nothing else of the person's
func runWithClaude(t *testing.T, script, answer, home string) string {
	t.Helper()
	bin := t.TempDir()
	if answer != "" {
		stub := "#!/bin/sh\n[ \"$1 $2 $3\" = 'auth status --json' ] || exit 2\ncat <<'EOF'\n" + answer + "EOF\n"
		if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + home}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return string(out)
}

func TestLoginScriptPassesOnTheEndOfTheSignInAlone(t *testing.T) {
	out := runWithClaude(t, LoginScript, authStatus, t.TempDir())
	if out != "login_expires=2026-12-05T16:18:16.555Z\nlogin_read=1\n" {
		t.Fatalf("the probe printed %q", out)
	}
	if strings.Contains(out, "@") || strings.Contains(out, "Organization") || strings.Contains(out, "max") {
		t.Fatalf("an account detail left the machine: %q", out)
	}
	login, read := ParseLogin(out, time.Unix(100, 0))
	if !read || login == nil || !login.Expires.Equal(time.Date(2026, 12, 5, 16, 18, 16, 555e6, time.UTC)) || login.At.Unix() != 100 {
		t.Fatalf("login %+v read %v", login, read)
	}
}

func TestLoginScriptOnOtherAnswers(t *testing.T) {
	compact := `{"loggedIn":true,"authMethod":"oauth_token","loginExpiresAt":"2027-01-02T03:04:05Z","email":"a@b.c"}` + "\n"
	if out := runWithClaude(t, LoginScript, compact, t.TempDir()); out != "login_expires=2027-01-02T03:04:05Z\nlogin_read=1\n" {
		t.Fatalf("a compact answer: %q", out)
	}
	apiKey := "{\n  \"loggedIn\": true,\n  \"authMethod\": \"api_key\",\n  \"loginExpiresAt\": null,\n  \"apiKeySource\": \"ANTHROPIC_API_KEY\"\n}\n"
	out := runWithClaude(t, LoginScript, apiKey, t.TempDir())
	if login, read := ParseLogin(out, time.Now()); login != nil || !read {
		t.Fatalf("an API key has no sign-in to end: %q", out)
	}
	before := "{\n  \"loggedIn\": true,\n  \"authMethod\": \"claude.ai\",\n  \"email\": \"a@b.c\"\n}\n" // a Claude Code before 2.1.318
	if login, read := ParseLogin(runWithClaude(t, LoginScript, before, t.TempDir()), time.Now()); login != nil || !read {
		t.Fatal("an answer without the field reads as no end known")
	}
	if login, read := ParseLogin(runWithClaude(t, LoginScript, "", t.TempDir()), time.Now()); login != nil || !read {
		t.Fatal("no Claude Code, nothing to read, and the read is done")
	}
	if _, read := ParseLogin("", time.Now()); read {
		t.Fatal("an empty answer is no read: the last one stands")
	}
	if login, _ := ParseLogin("login_expires=tomorrow\nlogin_read=1\n", time.Now()); login != nil {
		t.Fatal("a moment that does not parse is no end")
	}
}

func TestSettingsScriptReadsTheDefaultModelAndFastMode(t *testing.T) {
	home := t.TempDir()
	write := func(settings string) {
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out := runWithClaude(t, SettingsScript, "", home); out != "" {
		t.Fatalf("no settings, nothing said: %q", out)
	}
	write("{\n  \"model\": \"claude-opus-4-8\",\n  \"fastMode\": true,\n  \"agents\": {\n    \"x\": {\n      \"model\": \"haiku\"\n    }\n  }\n}\n")
	out := runWithClaude(t, SettingsScript, "", home)
	if out != "claude_model=claude-opus-4-8\nclaude_fast=on\n" {
		t.Fatalf("settings: %q", out)
	}
	if r, _ := Parse(out); r.ClaudeModel != "claude-opus-4-8" || !r.ClaudeFast {
		t.Fatalf("parsed %+v", r)
	}
	write("{\n  \"fastMode\": true,\n  \"fastModePerSessionOptIn\": true\n}\n")
	if out := runWithClaude(t, SettingsScript, "", home); out != "" {
		t.Fatalf("fast mode reset each session is not on for a new one: %q", out)
	}
	if !strings.Contains(Script(), SettingsScript) {
		t.Fatal("the probe does not read the settings")
	}
}

// asker is a stand-in for asking Claude Code on a machine: the answers it gives in turn, and how often it was asked
type asker struct {
	answers []error
	asked   int
}

func (a *asker) ask() (string, error) {
	a.asked++
	if a.asked <= len(a.answers) && a.answers[a.asked-1] != nil {
		return "", a.answers[a.asked-1]
	}
	return "login_expires=2026-12-05T16:18:16.555Z\nlogin_read=1\n", nil
}

func TestTheSignInIsReadSixHoursApartAndKeptAcrossARestart(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil, "v1", false)
	a := &asker{}
	if login := c.signIn("gog", false, a.ask); login == nil || a.asked != 1 {
		t.Fatalf("the first read asks: %+v, asked %d", login, a.asked)
	}
	if login := c.signIn("gog", false, a.ask); login == nil || a.asked != 1 {
		t.Fatalf("a probe soon after keeps the answer: asked %d", a.asked)
	}
	restarted := New(dir, nil, "v1", false)
	if login := restarted.signIn("gog", false, a.ask); login == nil || !login.Expires.Equal(time.Date(2026, 12, 5, 16, 18, 16, 555e6, time.UTC)) || a.asked != 1 {
		t.Fatalf("a restart asked Claude Code again: %+v, asked %d", login, a.asked)
	}
	if restarted.signIn("gog", true, a.ask); a.asked != 2 {
		t.Fatal("Read again asks whatever the age")
	}
	old := restarted.logins["gog"]
	old.At, old.Tried = time.Now().Add(-LoginEvery), time.Now().Add(-LoginEvery)
	restarted.logins["gog"] = old
	if restarted.signIn("gog", false, a.ask); a.asked != 3 {
		t.Fatal("an answer six hours old is asked for again")
	}
	ahead := restarted.logins["gog"]
	ahead.At, ahead.Tried = time.Now().Add(48*time.Hour), time.Now().Add(48*time.Hour)
	restarted.logins["gog"] = ahead
	if restarted.signIn("gog", false, a.ask); a.asked != 4 {
		t.Fatal("a read the clock was set back from counts as old")
	}
	if err := os.WriteFile(filepath.Join(dir, LoginsFile), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if broken := New(dir, nil, "v1", false); len(broken.logins) != 0 {
		t.Fatal("a damaged file reads as no sign-in known")
	}
}

func TestAReadThatFailsKeepsTheLastAnswerAndWaitsAnHour(t *testing.T) {
	c := New("", nil, "v1", false)
	a := &asker{answers: []error{nil, errors.New("timed out"), nil}}
	c.signIn("gog", false, a.ask)
	read := c.logins["gog"]
	read.At, read.Tried = time.Now().Add(-LoginEvery), time.Now().Add(-LoginEvery)
	c.logins["gog"] = read
	if login := c.signIn("gog", false, a.ask); login == nil || a.asked != 2 {
		t.Fatalf("a failed read keeps the last answer: %+v, asked %d", login, a.asked)
	}
	if c.signIn("gog", false, a.ask); a.asked != 2 {
		t.Fatal("a read that failed was asked again at the next probe")
	}
	failed := c.logins["gog"]
	failed.Tried = time.Now().Add(-LoginRetry)
	c.logins["gog"] = failed
	if login := c.signIn("gog", false, a.ask); login == nil || a.asked != 3 || time.Since(c.logins["gog"].At) > time.Minute {
		t.Fatalf("an hour after the failure it is asked again: %+v, asked %d", login, a.asked)
	}
}
