package limits

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const personsSettings = `{
  "model": "opus",
  "statusLine": {
    "type": "command",
    "command": "ccstatusline",
    "padding": 0,
    "refreshInterval": 10
  },
  "permissions": {
    "allow": ["Bash(git *)"]
  }
}
`

// homes is a temp HOME holding Claude Code's settings, and a state dir beside it, never the person's own.
func homes(t *testing.T, settings string) (home, stateDir string) {
	t.Helper()
	home, stateDir = t.TempDir(), t.TempDir()
	if settings != "" {
		must(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o700))
		must(t, os.WriteFile(SettingsPath(home), []byte(settings), 0o644))
	}
	return home, stateDir
}

func settingsOf(t *testing.T, home string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(SettingsPath(home))
	must(t, err)
	var out map[string]json.RawMessage
	must(t, json.Unmarshal(data, &out))
	return out
}

func TestInstallWrapsTheStatusLineAndKeepsEverythingElse(t *testing.T) {
	home, stateDir := homes(t, personsSettings)
	words, err := InstallStatusline(home, stateDir, "/Users/me/.local/bin/remuda")
	must(t, err)
	if !strings.Contains(words, "then runs ccstatusline") {
		t.Fatalf("words: %q", words)
	}
	data, _ := os.ReadFile(SettingsPath(home))
	text := string(data)
	if i, j, k := strings.Index(text, `"model"`), strings.Index(text, `"statusLine"`), strings.Index(text, `"permissions"`); !(i < j && j < k) {
		t.Fatalf("the settings' order moved:\n%s", text)
	}
	var line map[string]any
	must(t, json.Unmarshal(settingsOf(t, home)["statusLine"], &line))
	if line["command"] != `'/Users/me/.local/bin/remuda' statusline` || line["refreshInterval"] != float64(10) || line["type"] != "command" {
		t.Fatalf("status line: %v", line)
	}
	if string(settingsOf(t, home)["permissions"]) == "" {
		t.Fatal("a setting went missing")
	}
	backup, err := os.ReadFile(filepath.Join(stateDir, backupFile))
	if err != nil || string(backup) != personsSettings {
		t.Fatalf("backup: %v\n%s", err, backup)
	}
	if st, _ := os.Stat(SettingsPath(home)); st.Mode().Perm() != 0o644 {
		t.Fatalf("mode: %v", st.Mode())
	}
	if again, err := InstallStatusline(home, stateDir, "/Users/me/.local/bin/remuda"); err != nil || !strings.Contains(again, "already") {
		t.Fatalf("a second install: %q %v", again, err)
	}
	if status, _ := StatuslineStatus(home, stateDir); !strings.HasPrefix(status, "installed, then runs ccstatusline; no limits kept yet") {
		t.Fatalf("status: %q", status)
	}

	words, err = UninstallStatusline(home, stateDir)
	must(t, err)
	var back map[string]any
	must(t, json.Unmarshal(settingsOf(t, home)["statusLine"], &back))
	if back["command"] != "ccstatusline" || back["refreshInterval"] != float64(10) || !strings.Contains(words, "yours again") {
		t.Fatalf("after uninstall: %v %q", back, words)
	}
	if status, _ := StatuslineStatus(home, stateDir); !strings.HasPrefix(status, "not installed") {
		t.Fatalf("status: %q", status)
	}
}

func TestInstallWithoutAStatusLineAddsOneAndUninstallTakesItAway(t *testing.T) {
	home, stateDir := homes(t, "")
	words, err := InstallStatusline(home, stateDir, "/opt/remuda")
	must(t, err)
	if !strings.Contains(words, "prints nothing") {
		t.Fatalf("words: %q", words)
	}
	if _, ok := settingsOf(t, home)["statusLine"]; !ok {
		t.Fatal("no status line added")
	}
	_, err = UninstallStatusline(home, stateDir)
	must(t, err)
	if _, ok := settingsOf(t, home)["statusLine"]; ok {
		t.Fatal("the added status line stayed")
	}
}

func TestUninstallLeavesAStatusLineChangedSince(t *testing.T) {
	home, stateDir := homes(t, personsSettings)
	_, err := InstallStatusline(home, stateDir, "/opt/remuda")
	must(t, err)
	must(t, os.WriteFile(SettingsPath(home), []byte(`{"statusLine":{"type":"command","command":"starship statusline"}}`), 0o644))
	words, err := UninstallStatusline(home, stateDir)
	must(t, err)
	var line map[string]any
	must(t, json.Unmarshal(settingsOf(t, home)["statusLine"], &line))
	if line["command"] != "starship statusline" || !strings.Contains(words, "nothing changed") {
		t.Fatalf("%v %q", line, words)
	}
}

func TestInstallRefusesSettingsThatAreNotAnObject(t *testing.T) {
	home, stateDir := homes(t, `["not", "settings"]`)
	if _, err := InstallStatusline(home, stateDir, "/opt/remuda"); err == nil {
		t.Fatal("installed into a file that is not an object")
	}
	if data, _ := os.ReadFile(SettingsPath(home)); string(data) != `["not", "settings"]` {
		t.Fatalf("the file changed: %s", data)
	}
}

func TestTheWrapperKeepsTheLimitsAndRunsThePersonsStatusLine(t *testing.T) {
	home, stateDir := homes(t, `{"statusLine":{"type":"command","command":"cat > \"$OUT\"; printf 'line one\\nline two'"}}`)
	_, err := InstallStatusline(home, stateDir, "/opt/remuda")
	must(t, err)
	seen := filepath.Join(t.TempDir(), "seen.json")
	t.Setenv("OUT", seen)
	input := `{"model":{"display_name":"Opus"},"rate_limits":{"five_hour":{"used_percentage":12.5,"resets_at":1793801000},"seven_day":{"used_percentage":40,"resets_at":1794200000}}}`
	var out bytes.Buffer
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	RunStatusline(strings.NewReader(input), &out, stateDir, now)
	if out.String() != "line one\nline two" {
		t.Fatalf("printed %q", out.String())
	}
	if got, _ := os.ReadFile(seen); string(got) != input {
		t.Fatalf("the person's status line got %q", got)
	}
	data, err := os.ReadFile(filepath.Join(stateDir, SnapshotFile))
	must(t, err)
	windows := ClaudeWindows(data)
	if len(windows) != 2 || windows[0].Used != 12.5 || !windows[0].At.Equal(now) {
		t.Fatalf("kept: %+v", windows)
	}
	if st, _ := os.Stat(filepath.Join(stateDir, SnapshotFile)); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v", st.Mode())
	}
	RunStatusline(strings.NewReader(input), &bytes.Buffer{}, stateDir, now.Add(10*time.Second))
	if again, _ := os.ReadFile(filepath.Join(stateDir, SnapshotFile)); !bytes.Equal(again, data) {
		t.Fatal("an unchanged reading within the minute was written again")
	}
	if status, _ := StatuslineStatus(home, stateDir); !strings.Contains(status, "limits last kept") {
		t.Fatalf("status: %q", status)
	}
}

func TestTheWrapperNeverFails(t *testing.T) {
	_, stateDir := homes(t, "")
	var out bytes.Buffer
	RunStatusline(strings.NewReader("not json"), &out, stateDir, time.Now())
	if out.Len() != 0 {
		t.Fatalf("printed %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(stateDir, SnapshotFile)); err == nil {
		t.Fatal("kept something from nothing")
	}
	home, stateDir := homes(t, `{"statusLine":{"type":"command","command":"sleep 5"}}`)
	_, err := InstallStatusline(home, stateDir, "/opt/remuda")
	must(t, err)
	before := StatuslineTimeout
	StatuslineTimeout = 100 * time.Millisecond
	defer func() { StatuslineTimeout = before }()
	start := time.Now()
	RunStatusline(strings.NewReader(`{}`), &out, stateDir, time.Now())
	if time.Since(start) > 3*time.Second {
		t.Fatal("a slow status line held the wrapper")
	}
}
