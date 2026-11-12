package sessions

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDenyPromptsAddsTheSettingBeforeThePersonsArguments(t *testing.T) {
	args, err := StartArgs("claude", Start{Model: "opus", Args: []string{"--permission-mode", "acceptEdits"}, DenyPrompts: true})
	want := `--model|opus|--settings|{"permissions":{"denyInsteadOfPrompting":true}}|--permission-mode|acceptEdits`
	if err != nil || strings.Join(args, "|") != want {
		t.Fatalf("got %q %v, want %q", strings.Join(args, "|"), err, want)
	}
	resumed, err := StartArgs("claude", Start{Resume: "0123-abcd", DenyPrompts: true})
	if err != nil || strings.Join(resumed, "|") != `--resume|0123-abcd|--settings|{"permissions":{"denyInsteadOfPrompting":true}}` {
		t.Errorf("a resume keeps its place first: %q %v", resumed, err)
	}
	if plain, err := StartArgs("claude", Start{Args: []string{"--verbose"}}); err != nil || strings.Join(plain, "|") != "--verbose" {
		t.Errorf("off, nothing is added: %q %v", plain, err)
	}
}

func TestDenyPromptsMergesIntoThePersonsJSONSettings(t *testing.T) {
	cases := []struct {
		name string
		args []string
		at   int
	}{
		{"two words", []string{"--settings", `{"model":"opus","permissions":{"allow":["Bash(make *)"],"denyInsteadOfPrompting":false},"cleanupPeriodDays":30.5}`}, 1},
		{"one word", []string{"--verbose", `--settings={"model":"opus","permissions":{"allow":["Bash(make *)"]},"cleanupPeriodDays":30.5}`}, 1},
	}
	for _, c := range cases {
		args, err := StartArgs("claude", Start{Args: c.args, DenyPrompts: true})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(args) != len(c.args) {
			t.Fatalf("%s: a second --settings was added: %q", c.name, args)
		}
		value := strings.TrimPrefix(args[c.at], "--settings=")
		var got struct {
			Model       string
			Cleanup     json.Number `json:"cleanupPeriodDays"`
			Permissions struct {
				Allow []string
				Deny  bool `json:"denyInsteadOfPrompting"`
			}
		}
		if err := json.Unmarshal([]byte(value), &got); err != nil {
			t.Fatalf("%s: %q is no JSON: %v", c.name, value, err)
		}
		if got.Model != "opus" || got.Cleanup != "30.5" || len(got.Permissions.Allow) != 1 || got.Permissions.Allow[0] != "Bash(make *)" || !got.Permissions.Deny {
			t.Errorf("%s: the person's settings must stay and the setting be on: %q", c.name, value)
		}
		if strings.Contains(value, `>`) || !strings.Contains(value, "Bash(make *)") {
			t.Errorf("%s: written as the person wrote it: %q", c.name, value)
		}
	}
}

func TestDenyPromptsRefusesWhatItCannotMergeInto(t *testing.T) {
	for name, args := range map[string][]string{
		"a file":                {"--settings", "/home/me/.claude/unattended.json"},
		"a file in one word":    {"--settings=./settings.json"},
		"broken JSON":           {"--settings", `{"permissions":`},
		"permissions not a map": {"--settings", `{"permissions":["allow"]}`},
		"no value":              {"--verbose", "--settings"},
	} {
		if got, err := StartArgs("claude", Start{Args: args, DenyPrompts: true}); err == nil {
			t.Errorf("%s: refused, got %q", name, got)
		}
	}
	_, err := StartArgs("claude", Start{Args: []string{"--settings", "/home/me/s.json"}, DenyPrompts: true})
	if err == nil || !strings.Contains(err.Error(), "/home/me/s.json") || !strings.Contains(err.Error(), "denyInsteadOfPrompting") {
		t.Errorf("the refusal names the file and what to put in it: %v", err)
	}
	if _, err := StartArgs("codex", Start{DenyPrompts: true}); err == nil {
		t.Error("only Claude Code knows the setting")
	}
	if !CanDenyPrompts("claude") || CanDenyPrompts("codex") || CanDenyPrompts("gemini") || CanDenyPrompts("shell") {
		t.Error("CanDenyPrompts is Claude Code alone")
	}
}

func TestDenyPromptsLeavesWhatFollowsADoubleDashAlone(t *testing.T) {
	args, err := StartArgs("claude", Start{Args: []string{"--", "--settings", "not a flag here"}, DenyPrompts: true})
	want := `--settings|{"permissions":{"denyInsteadOfPrompting":true}}|--|--settings|not a flag here`
	if err != nil || strings.Join(args, "|") != want {
		t.Errorf("got %q %v", strings.Join(args, "|"), err)
	}
}
