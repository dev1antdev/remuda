package limits

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
)

const codexLine = `{"timestamp":"2026-11-04T10:00:00.000Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"limit_id":"codex","primary":{"used_percent":42.5,"window_minutes":300,"resets_at":1793800000},"secondary":{"used_percent":7.0,"window_minutes":10080,"resets_at":1794300000}}}}`

const claudeSnapshot = `{"at":"2026-11-04T10:05:00Z","rate_limits":{"five_hour":{"used_percentage":81.2,"resets_at":1793801000},"seven_day":{"used_percentage":30,"resets_at":1794200000},"spend_limit":{"used_percentage":5,"resets_at":1794200000}}}`

func TestParseReadsBothAgentsAndTheWrapper(t *testing.T) {
	now := time.Unix(1793790000, 0)
	out := codexLine + "\n" + script.RS + "\n" + claudeSnapshot + "\n" + script.RS + "\n" + "wrapped\n"
	r := Parse(out, now)
	if !r.Wrapped {
		t.Fatal("the wrapper was not read")
	}
	var got []string
	for _, w := range r.Windows {
		got = append(got, w.Agent+" "+w.Name)
	}
	if strings.Join(got, ", ") != "claude 5h, claude week, codex 5h, codex week" {
		t.Fatalf("windows: %v", got)
	}
	if w := r.Windows[0]; w.Used != 81.2 || w.ResetsAt.Unix() != 1793801000 || !w.At.Equal(time.Date(2026, 11, 4, 10, 5, 0, 0, time.UTC)) {
		t.Fatalf("claude 5h: %+v", w)
	}
	if w := r.Windows[2]; w.Used != 42.5 || w.ResetsAt.Unix() != 1793800000 || w.At.IsZero() {
		t.Fatalf("codex 5h: %+v", w)
	}
}

func TestParseDropsWindowsAlreadyReset(t *testing.T) {
	now := time.Unix(1793805000, 0) // past both 5-hour resets, before the weekly ones
	r := Parse(codexLine+"\n"+script.RS+"\n"+claudeSnapshot+"\n"+script.RS+"\n\n", now)
	for _, w := range r.Windows {
		if w.Name == "5h" {
			t.Fatalf("a window that reset still shows: %+v", w)
		}
	}
	if len(r.Windows) != 2 || r.Wrapped {
		t.Fatalf("reading: %+v", r)
	}
}

func TestParseTakesNothingFromNothing(t *testing.T) {
	if r := Parse("", time.Now()); len(r.Windows) != 0 || r.Wrapped {
		t.Fatalf("%+v", r)
	}
	if w := CodexWindows([]byte(`{"type":"event_msg","payload":{"type":"token_count","rate_limits":null}}`)); w != nil {
		t.Fatalf("codex without limits: %+v", w)
	}
	if w := ClaudeWindows([]byte(`{"at":"2026-11-04T10:05:00Z","rate_limits":{"five_hour":{"resets_at":1793801000}}}`)); w != nil {
		t.Fatalf("a window without its percentage: %+v", w)
	}
	if w := CodexWindows([]byte(`{"payload":{"rate_limits":{"primary":{"used_percent":1,"window_minutes":60,"resets_at":1793800000}}}}`)); len(w) != 1 || w[0].Name != "60m" {
		t.Fatalf("a window of another size: %+v", w)
	}
}

// TestScriptReadsTheMachine runs the script in a temp HOME: the newest rollout carrying limits, the wrapper's snapshot
// in remuda's state dir there, and the wrapper named in Claude Code's settings.
func TestScriptReadsTheMachine(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "11", "04")
	must(t, os.MkdirAll(day, 0o755))
	must(t, os.WriteFile(filepath.Join(day, "rollout-older.jsonl"), []byte(strings.Replace(codexLine, "42.5", "99", 1)+"\n"), 0o644))
	older := time.Now().Add(-time.Hour)
	must(t, os.Chtimes(filepath.Join(day, "rollout-older.jsonl"), older, older))
	must(t, os.WriteFile(filepath.Join(day, "rollout-newer.jsonl"), []byte(`{"type":"session_meta"}`+"\n"+codexLine+"\n"+`{"type":"response_item"}`+"\n"), 0o644))
	newest := filepath.Join(day, "rollout-newest-no-limits.jsonl")
	must(t, os.WriteFile(newest, []byte(`{"type":"session_meta"}`+"\n"), 0o644))
	soon := time.Now().Add(time.Minute)
	must(t, os.Chtimes(newest, soon, soon))
	stateDir := filepath.Join(home, ".config", "remuda")
	if runtime.GOOS == "darwin" {
		stateDir = filepath.Join(home, "Library", "Application Support", "remuda")
	}
	must(t, os.MkdirAll(stateDir, 0o700))
	must(t, os.WriteFile(filepath.Join(stateDir, SnapshotFile), []byte(claudeSnapshot), 0o600))
	must(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o700))
	must(t, os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"statusLine":{"type":"command","command":"'/x/.local/bin/remuda' statusline"}}`), 0o644))

	cmd := exec.Command("sh", "-c", Script())
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	r := Parse(string(out), time.Unix(1793790000, 0))
	if !r.Wrapped || len(r.Windows) != 4 {
		t.Fatalf("reading: %+v\n%s", r, out)
	}
	for _, w := range r.Windows {
		if w.Agent == "codex" && w.Name == "5h" && w.Used != 42.5 {
			t.Fatalf("not the newest rollout carrying limits: %+v", w)
		}
	}
}

func TestScriptAnswersAnEmptyMachine(t *testing.T) {
	cmd := exec.Command("sh", "-c", Script())
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	if r := Parse(string(out), time.Now()); len(r.Windows) != 0 || r.Wrapped {
		t.Fatalf("%+v", r)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
