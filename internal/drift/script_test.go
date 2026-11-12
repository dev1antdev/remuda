package drift

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// claudeJSON is an anonymized ~/.claude.json as Claude Code writes it: remuda registered for one project only, the
// user scope holding another server; the person's own keys are stand-ins.
const claudeJSON = `{
  "numStartups": 12,
  "projects": {
    "/home/someone/app": {
      "mcpServers": {
        "remuda": {
          "type": "stdio",
          "command": "/home/someone/.local/bin/remuda"
        }
      }
    }
  },
  "mcpServers": {
    "notes": {
      "type": "stdio",
      "command": "notes-mcp",
      "args": []
    }
  },
  "userID": "0000"
}
`

// geminiJSON is an anonymized ~/.gemini/settings.json with remuda among its servers, four spaces deep and a comment on top.
const geminiJSON = `// the person's settings
{
    "theme": "Default",

    "mcpServers": {
        "remuda": {
            "command": "/home/someone/.local/bin/remuda",
            "args": ["mcp"]
        },
        "other": {"command": "x"}
    }
}
`

const codexTOML = `model = "gpt-6"

[mcp_servers.notes]
command = "notes-mcp"

[mcp_servers.remuda] # added by remuda mcp install
command = "/home/someone/.local/bin/remuda"
args = ["mcp"]

[mcp_servers.remuda.env]
`

// herdrStub answers `herdr plugin list` with one plugin from GitHub and one linked from a folder.
const herdrStub = "#!/bin/sh\n[ \"$1 $2\" = \"plugin list\" ] && printf '2 plugins installed:\\n- herdr.push (Herdr Push) enabled [github:someone/herdr-push@f4fdb06d5413]\\n  config: /c\\n- local.one (Local) enabled [linked:/home/someone/plugins/one]\\n'\nexit 0\n"

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
}

func sha(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// TestScriptReadsTheConfigsAndCheckoutsOfATempHome runs the script on this machine's sh, awk, sed and git in a home of
// its own: the config files above, a herdr stand-in, and a repository with a linked worktree one commit behind its upstream.
func TestScriptReadsTheConfigsAndCheckoutsOfATempHome(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "CODEX_HOME=", "PATH=/usr/bin:/bin:"+os.Getenv("PATH"))
	write(t, filepath.Join(home, ".claude.json"), claudeJSON)
	write(t, filepath.Join(home, ".gemini/settings.json"), geminiJSON)
	write(t, filepath.Join(home, ".codex/config.toml"), codexTOML)
	write(t, filepath.Join(home, ".claude/CLAUDE.md"), "Be brief.\n")
	write(t, filepath.Join(home, ".local/bin/herdr"), herdrStub)
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=e2e@remuda.invalid", "-c", "user.name=e2e", "-c", "init.defaultBranch=main"}, args...)...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	remote, app, other := filepath.Join(home, "remote.git"), filepath.Join(home, "app"), filepath.Join(home, "other")
	git(home, "init", "-q", "--bare", remote)
	git(home, "init", "-q", app)
	write(t, filepath.Join(app, "CLAUDE.md"), "Run the tests.\n")
	git(app, "add", ".")
	git(app, "commit", "-q", "-m", "one")
	git(app, "remote", "add", "origin", remote)
	git(app, "push", "-q", "-u", "origin", "main")
	git(home, "clone", "-q", remote, other)
	write(t, filepath.Join(other, "AGENTS.md"), "More.\n")
	git(other, "add", ".")
	git(other, "commit", "-q", "-m", "two")
	git(other, "push", "-q")
	git(app, "fetch", "-q")
	git(app, "worktree", "add", "-q", "-b", "feature", filepath.Join(home, "wt"))
	git(app, "config", "remote.origin.url", "https://someone:s3cret@github.com/Owner/App.git")
	plain := filepath.Join(home, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "-c", Script([]string{filepath.Join(home, "wt"), app, plain, filepath.Join(home, "gone")}))
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the script failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "s3cret") {
		t.Fatal("the remote's token left the machine")
	}
	facts := Parse(string(out))
	if facts.MCP["claude"] || !facts.MCP["codex"] || !facts.MCP["gemini"] {
		t.Fatalf("mcp: %v (a project's own server is not the user scope's)", facts.MCP)
	}
	if _, read := facts.MCP["kimi"]; read {
		t.Fatal("Kimi's config was not there, yet it was read as one")
	}
	claude := facts.Instructions["claude"]
	if !claude.Present || claude.Hash != sha("Be brief.\n") || claude.Modified == 0 || facts.Instructions["codex"].Present || facts.Instructions["gemini"].Present {
		t.Fatalf("instructions: %+v", facts.Instructions)
	}
	if len(facts.Plugins) != 2 || facts.Plugins["herdr.push"] != "github:someone/herdr-push" || facts.Plugins["local.one"] != "linked" {
		t.Fatalf("plugins: %v", facts.Plugins)
	}
	if len(facts.Checkouts) != 1 {
		t.Fatalf("one repository behind two folders, none for a plain one: %+v", facts.Checkouts)
	}
	c := facts.Checkouts[0]
	realApp, _ := filepath.EvalSymlinks(app)
	if (c.Root != app && c.Root != realApp) || c.Origin != "https://github.com/Owner/App.git" || c.First != "" || c.Branch != "main" || c.Upstream != "origin/main" || c.Behind != 1 || c.Ahead != 0 || c.Fetched == 0 {
		t.Fatalf("checkout: %+v", c)
	}
	if f := c.Files["CLAUDE.md"]; !f.Present || f.Hash != sha("Run the tests.\n") || c.Files["AGENTS.md"].Present || c.Files[".claude/settings.json"].Present || len(c.Files) != 3 {
		t.Fatalf("files: %+v", c.Files)
	}
	if repoKey(c) != "github.com/Owner/App" || repoName(repoKey(c), c.Root) != "Owner/App" {
		t.Fatalf("key %q name %q", repoKey(c), repoName(repoKey(c), c.Root))
	}
}

func TestParseKeepsOnlyWhatItKnows(t *testing.T) {
	us := "\x1f"
	out := strings.Join([]string{
		"mcp" + us + "claude" + us + "yes",
		"mcp" + us + "copilot" + us + "yes",
		"mcp" + us + "codex",
		"global" + us + "codex" + us + us,
		"global" + us + "gemini" + us + "nothex" + us + "1700000000",
		"plugin bad/id github:x/y",
		"plugin ok.one",
		"repo" + us + "relative" + us + us + us + "main" + us + us + us + us,
		"repo" + us + "/r" + us + us + "abc123" + us + "main" + us + us + us + us,
		"repo" + us + "/r" + us + "dup" + us + us + "dev" + us + us + us + us,
		"file" + us + "/r" + us + "secret.env" + us + sha("x") + us + "1",
		"file" + us + "/nowhere" + us + "CLAUDE.md" + us + sha("x") + us + "1",
		"file" + us + "/r" + us + "AGENTS.md" + us + sha("x") + us + "1700000000",
	}, "\n")
	facts := Parse(out)
	if len(facts.MCP) != 1 || !facts.MCP["claude"] {
		t.Fatalf("mcp: %v", facts.MCP)
	}
	if facts.Instructions["codex"].Present || !facts.Instructions["gemini"].Present || facts.Instructions["gemini"].Hash != "" || facts.Instructions["gemini"].value() != "present" {
		t.Fatalf("instructions: %+v", facts.Instructions)
	}
	if len(facts.Plugins) != 0 {
		t.Fatalf("plugins: %v", facts.Plugins)
	}
	if len(facts.Checkouts) != 1 || facts.Checkouts[0].Branch != "main" || facts.Checkouts[0].First != "abc123" || len(facts.Checkouts[0].Files) != 1 || facts.Checkouts[0].Files["AGENTS.md"].Hash != sha("x") {
		t.Fatalf("checkouts: %+v", facts.Checkouts)
	}
	if repoKey(facts.Checkouts[0]) != "root:abc123" || repoName("root:abc123", "/home/x/notes") != "notes" || repoKey(Checkout{Root: "/x"}) != "" {
		t.Fatal("a checkout without a remote is named by its first commit and its folder")
	}
}
