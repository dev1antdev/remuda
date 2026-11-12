package drift

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/updates"
)

func offers(whats ...string) []updates.Update {
	var out []updates.Update
	for _, what := range whats {
		out = append(out, updates.Update{What: what, Title: "Title of " + what, Command: "run " + what})
	}
	return out
}

func checkout(behind int, claudeMD string) Checkout {
	return Checkout{Root: "/home/x/app", Origin: "git@github.com:owner/app.git", Branch: "main", Upstream: "origin/main", Behind: behind, Fetched: 1700000000,
		Files: map[string]File{"CLAUDE.md": {Present: true, Hash: sha(claudeMD), Modified: 1700000000}, "AGENTS.md": {}, ".claude/settings.json": {}}}
}

// fleet is three machines alike but for GOG, which lags in every way the table can tell, and one away.
func fleet() []Machine {
	report := func(claude string) *health.Report {
		return &health.Report{Claude: claude, ClaudeVia: "native", Codex: "0.80.0", CodexVia: "npm", Herdr: "0.9.6", HerdrServer: "0.9.6", Remuda: "v0.13.0", Statusline: true, ClaudeModel: "claude-opus-4-8",
			Integrations: map[string]string{"claude": "current", "codex": "current", "pi": "missing"}}
	}
	facts := func() *Facts {
		return &Facts{MCP: map[string]bool{"claude": true, "codex": true}, Instructions: map[string]File{"claude": {Present: true, Hash: sha("a"), Modified: 1}, "codex": {}}, Plugins: map[string]string{"herdr.push": "github:x/herdr-push"},
			Checkouts: []Checkout{checkout(0, "tests")}}
	}
	mac := Machine{ID: "local", Label: "This Mac", Report: report("2.1.320"), Facts: facts(), Offered: offers("herdr", "claude", "codex", "mcp")}
	gog := Machine{ID: "gog", Label: "GOG", Report: report("2.1.310"), Facts: facts(), Offered: offers("herdr", "claude", "codex", "hooks", "integrations", "mcp")}
	gog.Report.Integrations = map[string]string{"claude": "outdated", "codex": "missing"}
	gog.Report.ClaudeModel = ""
	gog.Facts.MCP["claude"] = false
	gog.Facts.Instructions["claude"] = File{Present: true, Hash: sha("b"), Modified: 2}
	gog.Facts.Plugins = map[string]string{}
	gog.Facts.Checkouts = []Checkout{checkout(3, "other tests")}
	pi := Machine{ID: "pi", Label: "Pi", Report: report("2.1.320"), Facts: facts(), Offered: offers("herdr", "claude", "mcp")}
	pi.Report.Codex, pi.Report.CodexVia = "", ""
	pi.Facts.Checkouts = []Checkout{{Root: "/home/x/notes", Origin: "https://github.com/owner/notes", Branch: "main", Files: map[string]File{}}}
	return []Machine{mac, gog, pi, {ID: "box", Label: "Box", Away: true}}
}

func find(t *testing.T, table Table, section, id, arg string) Row {
	t.Helper()
	for _, s := range table.Sections {
		if s.ID != section && s.Name != section {
			continue
		}
		for _, r := range s.Rows {
			if r.ID == id && r.Arg == arg {
				return r
			}
		}
	}
	t.Fatalf("no row %s %s in %s", id, arg, section)
	return Row{}
}

func has(table Table, section, id, arg string) bool {
	for _, s := range table.Sections {
		for _, r := range s.Rows {
			if (s.ID == section || s.Name == section) && r.ID == id && r.Arg == arg {
				return true
			}
		}
	}
	return false
}

func TestTheOddOneOutIsMarkedAndOfferedWhatBringsItInLine(t *testing.T) {
	table := Build(fleet(), time.Unix(1700000000, 0))
	if len(table.Machines) != 4 || !table.Machines[3].Away {
		t.Fatalf("machines: %+v", table.Machines)
	}
	claude := find(t, table, "agents", "agent.version", "claude")
	if claude.State != "odd" || claude.Norm != "2.1.320" || claude.Cells["gog"].Mark != "odd" || claude.Cells["local"].Mark != "" {
		t.Fatalf("claude: %+v", claude)
	}
	if a := claude.Cells["gog"].Action; a == nil || a.What != "claude" || a.Command != "run claude" || claude.Cells["local"].Action != nil {
		t.Fatalf("the lagging Claude Code is not offered its update: %+v", claude.Cells)
	}
	if _, ok := claude.Cells["box"]; ok {
		t.Fatal("an away machine has a cell")
	}
	codex := find(t, table, "agents", "agent.version", "codex")
	if codex.Norm != "0.80.0" || codex.Cells["pi"].Value != "" || codex.Cells["pi"].Mark != "odd" || codex.Cells["pi"].Action != nil {
		t.Fatalf("an agent missing on one machine is marked, with nothing to install it: %+v", codex)
	}
	if via := find(t, table, "agents", "agent.via", "codex"); via.State != "same" || len(via.Cells) != 2 {
		t.Fatalf("how an agent is installed counts only where it is: %+v", via)
	}
	if has(table, "agents", "agent.version", "kimi") {
		t.Fatal("an agent on no machine has a row")
	}
	hooks := find(t, table, "herdr", "herdr.hooks", "claude")
	if hooks.Cells["gog"].Mark != "odd" || hooks.Cells["gog"].Action == nil || hooks.Cells["gog"].Action.What != "hooks" {
		t.Fatalf("outdated hooks: %+v", hooks)
	}
	missing := find(t, table, "herdr", "herdr.hooks", "codex")
	if missing.Cells["gog"].Value != "missing" || missing.Cells["gog"].Action == nil || missing.Cells["gog"].Action.What != "integrations" || len(missing.Cells) != 2 {
		t.Fatalf("missing hooks, and none for a machine without Codex: %+v", missing)
	}
	if has(table, "herdr", "herdr.hooks", "pi") {
		t.Fatal("an integration for an agent remuda does not compare has a row")
	}
	if plugin := find(t, table, "herdr", "herdr.plugin", "herdr.push"); plugin.Cells["gog"].Mark != "odd" || plugin.Cells["gog"].Action != nil {
		t.Fatalf("a plugin missing on one machine: %+v", plugin)
	}
	mcp := find(t, table, "remuda", "mcp", "claude")
	if mcp.Norm != "yes" || mcp.Cells["gog"].Value != "no" || mcp.Cells["gog"].Action == nil || mcp.Cells["gog"].Action.What != "mcp" {
		t.Fatalf("remuda's MCP server: %+v", mcp)
	}
	if has(table, "remuda", "mcp", "gemini") {
		t.Fatal("an MCP row for an agent on no machine")
	}
	if model := find(t, table, "claude", "claude.model", ""); model.Cells["gog"].Value != "default" || model.Cells["gog"].Mark != "odd" || model.Cells["gog"].Action != nil {
		t.Fatalf("a default model is by hand: %+v", model)
	}
	if instructions := find(t, table, "instructions", "instructions", "claude"); instructions.Cells["gog"].Mark != "odd" || instructions.Cells["gog"].At != 2 || instructions.Cells["gog"].Action != nil {
		t.Fatalf("a different CLAUDE.md is by hand: %+v", instructions)
	}
	if has(table, "instructions", "instructions", "codex") {
		t.Fatal("an instruction file absent everywhere has a row")
	}
	if same := find(t, table, "herdr", "herdr.version", ""); same.State != "same" || same.Norm != "0.9.6" || same.Cells["gog"].Mark != "" || same.Cells["gog"].Action != nil || len(same.Hand) != 0 {
		t.Fatalf("a row in line marks and offers nothing: %+v", same)
	}
	for _, c := range []struct {
		section, id, arg string
		hand             []string
	}{
		{"agents", "agent.version", "claude", nil},
		{"agents", "agent.version", "codex", []string{"pi"}},
		{"remuda", "mcp", "claude", nil},
		{"claude", "claude.model", "", []string{"gog"}},
		{"instructions", "instructions", "claude", []string{"gog"}},
		{"herdr", "herdr.plugin", "herdr.push", []string{"gog"}},
		{"owner/app", "repo.behind", "", []string{"gog"}},
		{"owner/app", "repo.file", "CLAUDE.md", []string{"local", "gog"}},
	} {
		if row := find(t, table, c.section, c.id, c.arg); strings.Join(row.Hand, " ") != strings.Join(c.hand, " ") {
			t.Errorf("%s %s by hand on %v, want %v", c.id, c.arg, row.Hand, c.hand)
		}
	}
	machines := fleet()
	machines[1].Report.Claude = "2.1.330"
	newest := find(t, Build(machines, time.Now()), "agents", "agent.version", "claude")
	if newest.Norm != "2.1.320" || newest.Cells["gog"].Mark != "odd" || newest.Cells["local"].Action == nil || newest.Cells["pi"].Action == nil || len(newest.Hand) != 0 {
		t.Fatalf("the odd one out ahead of the rest: the others are offered their update, nothing by hand: %+v", newest)
	}
}

func TestRepositoriesAreComparedWhereTwoMachinesHoldThem(t *testing.T) {
	table := Build(fleet(), time.Now())
	var repo *Section
	for i := range table.Sections {
		if table.Sections[i].ID == "repo" {
			repo = &table.Sections[i]
		}
	}
	if repo == nil || repo.Name != "owner/app" || repo.Folders["gog"] != "/home/x/app" || len(repo.Folders) != 2 {
		t.Fatalf("repository: %+v", repo)
	}
	if len(table.Single) != 1 || table.Single[0].Name != "owner/notes" || table.Single[0].Machine != "pi" {
		t.Fatalf("a repository on one machine: %+v", table.Single)
	}
	behind := find(t, table, "owner/app", "repo.behind", "")
	if behind.Norm != "0" || behind.Cells["gog"].Mark != "odd" || behind.Cells["gog"].Note != "origin/main" || behind.Cells["gog"].At != 1700000000 {
		t.Fatalf("a checkout behind: %+v", behind)
	}
	if file := find(t, table, "owner/app", "repo.file", "CLAUDE.md"); file.State != "split" || file.Cells["local"].Mark != "split" || file.Cells["gog"].Mark != "split" {
		t.Fatalf("two different copies with no norm: %+v", file)
	}
	if has(table, "owner/app", "repo.file", "AGENTS.md") {
		t.Fatal("a file absent from every checkout has a row")
	}
	if branch := find(t, table, "owner/app", "repo.branch", ""); branch.State != "same" {
		t.Fatalf("branch: %+v", branch)
	}
}

func TestATieGoesToTheBetterValueWhereThereIsOne(t *testing.T) {
	machines := fleet()[:2]
	table := Build(machines, time.Now())
	claude := find(t, table, "agents", "agent.version", "claude")
	if claude.State != "odd" || claude.Norm != "2.1.320" || claude.Cells["gog"].Mark != "odd" {
		t.Fatalf("two versions, one newer: %+v", claude)
	}
	if model := find(t, table, "claude", "claude.model", ""); model.State != "split" || model.Norm != "" {
		t.Fatalf("two models, neither better: %+v", model)
	}
	machines[0].Report.Claude = "2.1.300"
	if newest := find(t, Build(machines, time.Now()), "agents", "agent.version", "claude"); newest.Cells["local"].Action == nil || newest.Cells["gog"].Action != nil {
		t.Fatalf("the older one is offered the update, whichever machine it is: %+v", newest.Cells)
	}
	alone := Build(machines[:1], time.Now())
	if row := find(t, alone, "agents", "agent.version", "claude"); row.State != "single" || row.Cells["local"].Mark != "" {
		t.Fatalf("one machine has nothing to differ from: %+v", row)
	}
}

func TestAMachineNotReadHasNoCells(t *testing.T) {
	machines := fleet()
	machines[1].Facts, machines[1].Error = nil, "ssh: timeout"
	machines[2].Report = &health.Report{Error: "unreachable"}
	table := Build(machines, time.Now())
	if table.Machines[1].Error != "ssh: timeout" {
		t.Fatalf("columns: %+v", table.Machines)
	}
	mcp := find(t, table, "remuda", "mcp", "claude")
	if _, ok := mcp.Cells["gog"]; ok || mcp.State != "single" {
		t.Fatalf("a machine the script failed on still has a fact of it: %+v", mcp)
	}
	if claude := find(t, table, "agents", "agent.version", "claude"); len(claude.Cells) != 2 {
		t.Fatalf("its vitals still stand, a failed probe's do not: %+v", claude.Cells)
	}
	if empty := Build(nil, time.Now()); len(empty.Machines) != 0 || len(empty.Sections) != 0 {
		t.Fatalf("no machine: %+v", empty)
	}
}
