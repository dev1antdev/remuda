package coreapi

import (
	"strings"
	"testing"
)

func TestAnAgentIsKnownByItsProgramOrTheScriptAnInterpreterRuns(t *testing.T) {
	for argv, want := range map[string]string{
		"claude":                            "claude",
		`C:\Users\me\.local\bin\claude.exe`: "claude",
		"node --inspect /opt/lib/gemini":    "gemini",
		"python3 -u /home/me/kimi":          "kimi",
		"claude-code":                       "claude",
		"node /opt/lib/claude-code/cli.js":  "",
		"vim claude":                        "",
		"zsh":                               "",
	} {
		if got := AgentProgram(strings.Fields(argv)); got != want {
			t.Errorf("%q runs %q, want %q", argv, got, want)
		}
	}
}

func TestAnAgentKindStartsAsItsProgram(t *testing.T) {
	for kind, want := range map[string]string{"claude": "claude", "codex": "codex", "cursor": "cursor-agent", "qwen": "qwen"} {
		if got, ok := AgentCommand(kind); !ok || got != want {
			t.Errorf("%s starts as %q %v, want %q", kind, got, ok, want)
		}
	}
	if _, ok := AgentCommand("vim"); ok {
		t.Error("a program that is no agent")
	}
}
