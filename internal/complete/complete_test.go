package complete

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestBuiltinsAreEachAgentsOwn(t *testing.T) {
	claude := Builtins("claude")
	if len(claude) == 0 || claude[0].Source != "builtin" {
		t.Fatalf("claude: %+v", claude)
	}
	has := func(list []Command, name string) bool {
		for _, c := range list {
			if c.Name == name {
				return true
			}
		}
		return false
	}
	if !has(claude, "compact") || !has(claude, "model") || !has(claude, "restart") {
		t.Fatalf("claude lacks a command it has: %+v", claude)
	}
	if codex := Builtins("codex"); !has(codex, "compact") || has(codex, "btw") {
		t.Fatalf("codex: %+v", codex)
	}
	if gemini := Builtins("gemini"); !has(gemini, "compress") || has(gemini, "compact") {
		t.Fatalf("gemini: %+v", gemini)
	}
	if kimi := Builtins("kimi"); len(kimi) != 0 {
		t.Fatalf("kimi should list nothing: %+v", kimi)
	}
	if shell := Builtins(""); len(shell) != 0 {
		t.Fatalf("a shell should list nothing: %+v", shell)
	}
}

const recorded = "user" + script.US + "/home/me/.claude/commands/fix.md\n" +
	"---\n" +
	"description: \"Fix the failing test and say why it failed\"\n" +
	"allowed-tools: Bash\n" +
	"---\n" +
	"# Fix\n" +
	script.GS + "\n" +
	"user" + script.US + "/home/me/.claude/commands/sub/plain.md\n" +
	"# Plain command\n" +
	"\n" +
	"Its body.\n" +
	script.GS + "\n" +
	"user" + script.US + "/home/me/.claude/skills/verify/SKILL.md\n" +
	"---\n" +
	"name: verify\n" +
	"description: Run the full check before calling work done\n" +
	"---\n" +
	"# Verify\n" +
	script.GS + "\n" +
	"project" + script.US + "/repo/.claude/skills/look/SKILL.md\n" +
	"A skill with no frontmatter: its first line says what it does, and it goes on and on and on and on and on and on and on and on and on and on and on and on past the cut.\n" +
	script.GS + "\n" +
	"project" + script.US + "/repo/.claude/commands/empty.md\n" +
	script.GS + "\n" +
	"garbage line\n" +
	script.GS + "\n"

func TestParseCommandsReadsNamesAndDescriptions(t *testing.T) {
	list := ParseCommands(recorded)
	if len(list) != 5 {
		t.Fatalf("want 5 commands, got %d: %+v", len(list), list)
	}
	want := []Command{
		{Name: "fix", Description: "Fix the failing test and say why it failed", Source: "user"},
		{Name: "plain", Description: "Plain command", Source: "user"},
		{Name: "verify", Description: "Run the full check before calling work done", Source: "user"},
		{Name: "look", Source: "project"},
		{Name: "empty", Source: "project"},
	}
	for i, w := range want {
		got := list[i]
		if got.Name != w.Name || got.Source != w.Source {
			t.Fatalf("command %d: want %+v, got %+v", i, w, got)
		}
		if w.Description != "" && got.Description != w.Description {
			t.Fatalf("command %d: want description %q, got %q", i, w.Description, got.Description)
		}
	}
	if long := list[3].Description; !strings.HasSuffix(long, "…") || len([]rune(long)) > MaxDescription+1 || !strings.HasPrefix(long, "A skill with no frontmatter") {
		t.Fatalf("a long first line is cut at a word: %q", long)
	}
}

func TestCommandsRunTheScriptOncePerMinute(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	svc := New()
	svc.now = func() time.Time { return now }
	host := &shell.Fake{Name: "box", Dir: "/home/me", Answer: func(string) (string, error) { return recorded, nil }}

	first, err := svc.Commands(host, "/repo", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(Builtins("claude"))+5 || first[0].Source != "builtin" || first[len(first)-1].Name != "empty" {
		t.Fatalf("builtins first, then the files: %d commands, first %+v, last %+v", len(first), first[0], first[len(first)-1])
	}
	scripts := host.Scripts()
	if len(scripts) != 1 || !strings.Contains(scripts[0], "'/repo'") || !strings.Contains(scripts[0], ".claude/commands") || !strings.Contains(scripts[0], "SKILL.md") {
		t.Fatalf("the one script names the folder, the commands and the skills: %q", scripts)
	}
	if _, err := svc.Commands(host, "/repo", "claude"); err != nil || len(host.Scripts()) != 1 {
		t.Fatalf("a second read within the minute runs nothing: %d scripts", len(host.Scripts()))
	}
	now = now.Add(CacheFor + time.Second)
	if _, err := svc.Commands(host, "/repo", "claude"); err != nil || len(host.Scripts()) != 2 {
		t.Fatalf("after the minute the folder is read again: %d scripts", len(host.Scripts()))
	}
	if codex, err := svc.Commands(host, "/repo", "codex"); err != nil || len(host.Scripts()) != 2 || len(codex) != len(Builtins("codex")) {
		t.Fatalf("codex gets its built-ins and no script: %d scripts, %d commands", len(host.Scripts()), len(codex))
	}
}

func TestCommandsReportAHostAway(t *testing.T) {
	svc := New()
	host := &shell.Fake{Name: "box", Remote: true, Answer: func(string) (string, error) { return "", errors.New("ssh: connect to host box: no route") }}
	if _, err := svc.Commands(host, "/repo", "claude"); err == nil {
		t.Fatal("a failed script is an error, not an empty list")
	}
}

func TestFilesListTheFolderAndNoteACut(t *testing.T) {
	svc := New()
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "a.go\nweb/src/x.ts\n\n.claude/commands/fix.md\n", nil }}
	files, err := svc.Files(host, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(files.Paths) != 3 || files.Paths[1] != "web/src/x.ts" || files.Cut {
		t.Fatalf("files: %+v", files)
	}
	if s := host.Scripts(); len(s) != 1 || !strings.HasPrefix(s[0], "cd '/repo'") || !strings.Contains(s[0], "ls-files") || !strings.Contains(s[0], "find .") {
		t.Fatalf("the script enters the folder and lists it with git or find: %q", s)
	}
	if _, err := svc.Files(host, "/repo"); err != nil || len(host.Scripts()) != 1 {
		t.Fatalf("the list is kept a minute: %d scripts", len(host.Scripts()))
	}

	var many []string
	for i := 0; i <= MaxPaths; i++ {
		many = append(many, "f"+strings.Repeat("x", i%7))
	}
	big := ParsePaths(strings.Join(many, "\n") + "\n")
	if len(big.Paths) != MaxPaths || !big.Cut {
		t.Fatalf("a folder past MaxPaths is cut and says so: %d, cut %v", len(big.Paths), big.Cut)
	}
}
