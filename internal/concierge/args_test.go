package concierge

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/sessions"
)

func TestSettingsAllowRemudasToolsAloneAndDenyTheRest(t *testing.T) {
	var got struct {
		Permissions struct {
			Allow       []string
			Deny        []string
			DefaultMode string `json:"defaultMode"`
			NoPrompts   bool   `json:"denyInsteadOfPrompting"`
		}
	}
	if err := json.Unmarshal([]byte(Settings()), &got); err != nil {
		t.Fatalf("%q is no JSON: %v", Settings(), err)
	}
	p := got.Permissions
	want := []string{"mcp__remuda__agents_list", "mcp__remuda__agent_recap", "mcp__remuda__standup", "mcp__remuda__tasks_list", "mcp__remuda__task_get", "mcp__remuda__projects_list", "mcp__remuda__labels_list", "mcp__remuda__task_create", "mcp__remuda__task_comment"}
	if !slices.Equal(p.Allow, want) {
		t.Errorf("allowed %q, want remuda's reads, task_create and task_comment alone", p.Allow)
	}
	for _, tool := range []string{"mcp__remuda__task_update", "Bash", "PowerShell", "Read", "Write", "Edit", "MultiEdit", "NotebookEdit", "Glob", "Grep", "WebFetch", "WebSearch", "Agent", "Task", "Skill", "EnterWorktree", "CronCreate"} {
		if !slices.Contains(p.Deny, tool) {
			t.Errorf("%s is not denied", tool)
		}
	}
	for _, tool := range p.Allow {
		if slices.Contains(p.Deny, tool) {
			t.Errorf("%s is both allowed and denied", tool)
		}
	}
	if p.DefaultMode != "dontAsk" || !p.NoPrompts {
		t.Errorf("anything not allowed is denied, never asked: mode %q, denyInsteadOfPrompting %v", p.DefaultMode, p.NoPrompts)
	}
	if strings.Contains(Settings(), "\n") || strings.Contains(Settings(), `>`) {
		t.Errorf("one line, as written: %q", Settings())
	}
}

func TestMCPConfigNamesRemudasOwnServerAlone(t *testing.T) {
	bin := `/Users/o'neil/My Tools/remuda`
	var got struct {
		MCPServers map[string]struct {
			Type    string
			Command string
			Args    []string
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(MCPConfig(bin)), &got); err != nil {
		t.Fatalf("%q is no JSON: %v", MCPConfig(bin), err)
	}
	server, ok := got.MCPServers["remuda"]
	if len(got.MCPServers) != 1 || !ok || server.Type != "stdio" || server.Command != bin || !slices.Equal(server.Args, []string{"mcp"}) {
		t.Errorf("one server, remuda's `mcp` at its own path: %+v", got)
	}
}

func TestArgsComeAfterTheResumeWithTheMCPConfigLast(t *testing.T) {
	args, err := sessions.StartArgs(Agent, sessions.Start{Resume: "0a1b2c3d-1111-2222-3333-444455556666", Args: Args("/home/me/.local/bin/remuda")})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 7 || args[0] != "--resume" || args[1] != "0a1b2c3d-1111-2222-3333-444455556666" || args[2] != "--settings" || args[3] != Settings() || args[4] != "--strict-mcp-config" || args[5] != "--mcp-config" || args[6] != MCPConfig("/home/me/.local/bin/remuda") {
		t.Errorf("got %q", args)
	}
	fresh, _ := sessions.StartArgs(Agent, sessions.Start{Args: Args("/x/remuda")})
	if fresh[0] != "--settings" || fresh[len(fresh)-2] != "--mcp-config" {
		t.Errorf("a fresh one starts on its settings and ends on its one server: %q", fresh)
	}
}
