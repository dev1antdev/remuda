package drift

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/teleport"
)

// agents are the coding agents the table compares, in the order every list of them keeps.
var agents = []string{"claude", "codex", "gemini", "kimi"}

// instructed are the agents whose global instruction file is known: ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md, ~/.gemini/GEMINI.md.
var instructed = []string{"claude", "codex", "gemini"}

func agentVersion(r *health.Report, agent string) (version, via string) {
	switch agent {
	case "claude":
		return r.Claude, r.ClaudeVia
	case "codex":
		return r.Codex, r.CodexVia
	case "gemini":
		return r.Gemini, r.GeminiVia
	case "kimi":
		return r.Kimi, r.KimiVia
	}
	return "", ""
}

// reported is a cell read off the machine's vitals; none before its first reading.
func reported(value func(r *health.Report) (string, bool)) func(m Machine) (Cell, bool) {
	return func(m Machine) (Cell, bool) {
		if m.Report == nil || m.Report.Error != "" {
			return Cell{}, false
		}
		v, ok := value(m.Report)
		return Cell{Value: v}, ok
	}
}

func installed(r *health.Report, agent string) bool {
	version, _ := agentVersion(r, agent)
	return version != ""
}

func flag(on bool) string {
	if on {
		return "yes"
	}
	return "no"
}

// rows builds every spec that holds something.
func rows(machines []Machine, specs []spec) []Row {
	var out []Row
	for _, s := range specs {
		if row, ok := s.build(machines); ok {
			out = append(out, row)
		}
	}
	return out
}

func agentsSection(machines []Machine) Section {
	var specs []spec
	for _, agent := range agents {
		specs = append(specs,
			spec{id: "agent.version", arg: agent, kind: "version", better: newer,
				cell:   reported(func(r *health.Report) (string, bool) { v, _ := agentVersion(r, agent); return v, true }),
				action: func(Machine, string) string { return agent }},
			spec{id: "agent.via", arg: agent, kind: "via",
				cell: reported(func(r *health.Report) (string, bool) {
					v, via := agentVersion(r, agent)
					if via == "" {
						via = "other"
					}
					return via, v != ""
				})})
	}
	return Section{ID: "agents", Rows: rows(machines, specs)}
}

func herdrSection(machines []Machine) Section {
	specs := []spec{
		{id: "herdr.version", kind: "version", better: newer, cell: reported(func(r *health.Report) (string, bool) { return r.Herdr, true }),
			action: func(Machine, string) string { return "herdr" }},
		{id: "herdr.server", kind: "version", better: newer, cell: reported(func(r *health.Report) (string, bool) { return r.HerdrServer, r.Herdr != "" }),
			action: func(Machine, string) string { return "restart-herdr" }},
	}
	for _, agent := range agents {
		specs = append(specs, spec{id: "herdr.hooks", arg: agent, kind: "hook", better: hookRank,
			cell: reported(func(r *health.Report) (string, bool) {
				state, ok := r.Integrations[agent]
				return state, ok && r.Herdr != "" && installed(r, agent)
			}),
			action: func(_ Machine, value string) string {
				if value == "missing" {
					return "integrations"
				}
				return "hooks"
			}})
	}
	plugins := map[string]bool{}
	for _, m := range machines {
		if m.Facts != nil {
			for id := range m.Facts.Plugins {
				plugins[id] = true
			}
		}
	}
	for _, id := range sortedKeys(plugins) {
		specs = append(specs, spec{id: "herdr.plugin", arg: id, kind: "plugin", better: present,
			cell: func(m Machine) (Cell, bool) {
				if m.Facts == nil || m.Report == nil || m.Report.Herdr == "" {
					return Cell{}, false
				}
				return Cell{Value: m.Facts.Plugins[id]}, true
			}})
	}
	return Section{ID: "herdr", Rows: rows(machines, specs)}
}

func remudaSection(machines []Machine) Section {
	specs := []spec{
		{id: "remuda.version", kind: "version", better: newer, cell: reported(func(r *health.Report) (string, bool) { return r.Remuda, true }),
			action: func(m Machine, _ string) string {
				if offered(m, "remuda-release") != nil {
					return "remuda-release"
				}
				return "remuda"
			}},
		{id: "code.version", kind: "version", better: newer, cell: reported(func(r *health.Report) (string, bool) { return r.CodeServer, true }),
			action: func(Machine, string) string { return "code" }},
	}
	for _, agent := range agents {
		specs = append(specs, spec{id: "mcp", arg: agent, kind: "flag", better: yes,
			cell: func(m Machine) (Cell, bool) {
				if m.Facts == nil || m.Report == nil || !installed(m.Report, agent) {
					return Cell{}, false
				}
				on, read := m.Facts.MCP[agent]
				return Cell{Value: flag(on)}, read
			},
			action: func(Machine, string) string { return "mcp" }})
	}
	specs = append(specs, spec{id: "statusline", kind: "flag", better: yes,
		cell:   reported(func(r *health.Report) (string, bool) { return flag(r.Statusline), r.Claude != "" }),
		action: func(Machine, string) string { return "statusline" }})
	return Section{ID: "remuda", Rows: rows(machines, specs)}
}

func claudeSection(machines []Machine) Section {
	return Section{ID: "claude", Rows: rows(machines, []spec{
		{id: "claude.model", kind: "model", cell: reported(func(r *health.Report) (string, bool) {
			model := r.ClaudeModel
			if model == "" {
				model = "default"
			}
			return model, r.Claude != ""
		})},
		{id: "claude.fast", kind: "flag", cell: reported(func(r *health.Report) (string, bool) { return flag(r.ClaudeFast), r.Claude != "" })},
	})}
}

func instructionsSection(machines []Machine) Section {
	var specs []spec
	for _, agent := range instructed {
		specs = append(specs, spec{id: "instructions", arg: agent, kind: "hash",
			cell: func(m Machine) (Cell, bool) {
				if m.Facts == nil || m.Report == nil || !installed(m.Report, agent) {
					return Cell{}, false
				}
				f, read := m.Facts.Instructions[agent]
				return Cell{Value: f.value(), At: f.Modified}, read
			}})
	}
	return Section{ID: "instructions", Rows: rows(machines, specs)}
}

// placed is a checkout on a machine, under its repository's key.
type placed struct {
	machine  Machine
	checkout Checkout
}

// repoKey names a repository the same on every machine: its remote however it is spelled, else its first commit.
func repoKey(c Checkout) string {
	if key := teleport.RemoteKey(c.Origin); key != "" {
		return key
	}
	if c.First != "" {
		return "root:" + c.First
	}
	return ""
}

// repoName is what a person calls the repository: the remote's owner/repo, else its folder's name.
func repoName(key, root string) string {
	if !strings.HasPrefix(key, "root:") {
		parts := strings.Split(key, "/")
		if len(parts) >= 2 {
			return strings.Join(parts[len(parts)-2:], "/")
		}
	}
	return path.Base(root)
}

// repoSections is a section per repository checked out on two machines or more, by name; the rest named alone.
func repoSections(machines []Machine) ([]Section, []Lone) {
	byKey := map[string][]placed{}
	for _, m := range machines {
		if m.Facts == nil {
			continue
		}
		seen := map[string]bool{}
		for _, c := range m.Facts.Checkouts {
			key := repoKey(c)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			byKey[key] = append(byKey[key], placed{m, c})
		}
	}
	var sections []Section
	var single []Lone
	for _, key := range sortedKeys(byKey) {
		list := byKey[key]
		name := repoName(key, list[0].checkout.Root)
		if len(list) < 2 {
			single = append(single, Lone{Name: name, Machine: list[0].machine.ID, Folder: list[0].checkout.Root})
			continue
		}
		section := Section{ID: "repo", Name: name, Folders: map[string]string{}}
		var on []Machine
		checkouts := map[string]Checkout{}
		for _, p := range list {
			section.Folders[p.machine.ID] = p.checkout.Root
			checkouts[p.machine.ID] = p.checkout
			on = append(on, p.machine)
		}
		specs := []spec{
			{id: "repo.branch", kind: "text", cell: func(m Machine) (Cell, bool) { return Cell{Value: checkouts[m.ID].Branch}, true }},
			{id: "repo.behind", kind: "behind", better: fewer, cell: func(m Machine) (Cell, bool) {
				c := checkouts[m.ID]
				return Cell{Value: strconv.Itoa(c.Behind), At: c.Fetched, Note: c.Upstream}, c.Upstream != ""
			}},
		}
		for _, file := range RepoFiles {
			specs = append(specs, spec{id: "repo.file", arg: file, kind: "hash", cell: func(m Machine) (Cell, bool) {
				f := checkouts[m.ID].Files[file]
				return Cell{Value: f.value(), At: f.Modified}, true
			}})
		}
		section.Rows = rows(on, specs)
		sections = append(sections, section)
	}
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].Name < sections[j].Name })
	sort.SliceStable(single, func(i, j int) bool { return single[i].Name < single[j].Name })
	return sections, single
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
