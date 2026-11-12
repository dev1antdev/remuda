import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import type { FleetRow, FleetTable } from "./api";
import { bringInLine, byHand, cellTip, differs, handWords, rowExplain, rowLabel, sectionTitle, shownSections, summaryWords, valueWords } from "./fleet";

afterEach(() => setLang("en"));

const now = Date.parse("2026-11-12T12:00:00Z");
const hour = 3600;
const at = now / 1000;
const update = (what: string) => ({ what, title: `Title of ${what}`, command: `run ${what}` });

// table is three machines where GOG lags: an older Claude Code with its update, remuda's MCP missing with its install,
// a CLAUDE.md of its own and a checkout behind, both by hand; Box is away
const table: FleetTable = {
  at: "2026-11-12T12:00:00Z",
  machines: [
    { id: "local", label: "This Mac" },
    { id: "gog", label: "GOG" },
    { id: "pi", label: "Pi" },
    { id: "box", label: "Box", away: true },
  ],
  sections: [
    {
      id: "agents",
      rows: [
        { id: "agent.version", arg: "claude", kind: "version", state: "odd", norm: "2.1.320", cells: { local: { value: "2.1.320" }, gog: { value: "2.1.310", mark: "odd", action: update("claude") }, pi: { value: "2.1.320" } } },
        { id: "agent.via", arg: "claude", kind: "via", state: "same", norm: "native", cells: { local: { value: "native" }, gog: { value: "native" }, pi: { value: "native" } } },
        { id: "agent.version", arg: "codex", kind: "version", state: "odd", norm: "0.80.0", cells: { local: { value: "0.80.0" }, gog: { value: "0.80.0" }, pi: { value: "", mark: "odd" } }, hand: ["pi"] },
      ],
    },
    {
      id: "remuda",
      rows: [
        { id: "mcp", arg: "claude", kind: "flag", state: "odd", norm: "yes", cells: { local: { value: "yes" }, gog: { value: "no", mark: "odd", action: update("mcp") }, pi: { value: "yes" } } },
        { id: "mcp", arg: "codex", kind: "flag", state: "odd", norm: "yes", cells: { local: { value: "yes" }, gog: { value: "no", mark: "odd", action: update("mcp") } } },
      ],
    },
    {
      id: "instructions",
      rows: [{ id: "instructions", arg: "claude", kind: "hash", state: "odd", norm: "a".repeat(64), cells: { local: { value: "a".repeat(64), at: at - 48 * hour }, gog: { value: "b".repeat(64), mark: "odd", at: at - 2 * hour }, pi: { value: "a".repeat(64), at: at - 72 * hour } }, hand: ["gog"] }],
    },
    {
      id: "repo",
      name: "owner/app",
      folders: { local: "/Users/x/app", gog: "/home/x/app" },
      rows: [
        { id: "repo.branch", kind: "text", state: "same", norm: "main", cells: { local: { value: "main" }, gog: { value: "main" } } },
        { id: "repo.behind", kind: "behind", state: "odd", norm: "0", cells: { local: { value: "0", at: at - hour, note: "origin/main" }, gog: { value: "3", mark: "odd", at: at - 5 * hour, note: "origin/main" } }, hand: ["gog"] },
        { id: "repo.file", arg: "CLAUDE.md", kind: "hash", state: "split", cells: { local: { value: "c".repeat(64), mark: "split", at: at - hour }, gog: { value: "d".repeat(64), mark: "split", at: at - 30 * hour } }, hand: ["local", "gog"] },
      ],
    },
  ],
  single: [{ name: "owner/notes", machine: "pi", folder: "/home/x/notes" }],
};

const names: Record<string, string> = { local: "This Mac", gog: "GOG", pi: "Pi", box: "Box" };
const name = (id: string) => names[id] ?? id;
const row = (section: number, index: number): FleetRow => table.sections[section]!.rows[index]!;

describe("the Fleet setup's words", () => {
  it("names the sections, the rows and their values", () => {
    expect(table.sections.map(sectionTitle)).toEqual(["Agents", "remuda", "Global instructions", "owner/app"]);
    expect(rowLabel(row(0, 0))).toBe("Claude Code");
    expect(rowLabel(row(1, 1))).toBe("Tasks for Codex (MCP)");
    expect(rowLabel(row(2, 0))).toBe("~/.claude/CLAUDE.md");
    expect(rowLabel(row(3, 2))).toBe("CLAUDE.md");
    expect(rowExplain(row(1, 0))).toContain("registered in Claude Code's own config");
    expect(valueWords(row(0, 2), row(0, 2).cells.pi!)).toBe("not installed");
    expect(valueWords(row(0, 1), { value: "other" })).toBe("another way");
    expect(valueWords(row(1, 0), { value: "no" })).toBe("no");
    expect(valueWords({ id: "claude.fast", kind: "flag", state: "same", cells: {} }, { value: "yes" })).toBe("on");
    expect(valueWords({ id: "herdr.server", kind: "version", state: "same", cells: {} }, { value: "" })).toBe("not running");
    expect(valueWords({ id: "herdr.plugin", arg: "p", kind: "plugin", state: "same", cells: {} }, { value: "github:someone/herdr-push" })).toBe("someone/herdr-push");
    expect(valueWords({ id: "herdr.plugin", arg: "p", kind: "plugin", state: "same", cells: {} }, { value: "linked" })).toBe("local");
    expect(valueWords(row(2, 0), row(2, 0).cells.gog!)).toBe("bbbbbbb");
    expect(valueWords(row(2, 0), { value: "" })).toBe("absent");
    expect(valueWords(row(3, 1), row(3, 1).cells.gog!)).toBe("3 commits behind");
    expect(valueWords(row(3, 1), row(3, 1).cells.local!)).toBe("up to date");
    expect(valueWords({ id: "claude.model", kind: "model", state: "same", cells: {} }, { value: "default" })).toBe("Claude Code's default");
    setLang("ru");
    expect(valueWords(row(3, 1), row(3, 1).cells.gog!)).toBe("отстаёт на 3 коммита");
    expect(rowLabel(row(1, 1))).toBe("Задачи для Codex (MCP)");
  });
  it("says a file's hash and age, a checkout's upstream and fetch", () => {
    expect(cellTip(row(2, 0), row(2, 0).cells.gog!, now)).toBe(`sha256 ${"b".repeat(64)}, changed 2 h ago`);
    expect(cellTip(row(3, 1), row(3, 1).cells.gog!, now)).toBe("against origin/main, fetched 5 h ago");
    expect(cellTip(row(3, 1), { value: "0", note: "origin/dev" }, now)).toBe("against origin/dev, never fetched there");
    expect(cellTip(row(0, 0), row(0, 0).cells.gog!, now)).toBe("");
  });
  it("shows the rows that differ unless asked for all, and counts them", () => {
    expect(differs(row(0, 1))).toBe(false);
    expect(differs(row(3, 2))).toBe(true);
    expect(shownSections(table, false).map((s) => s.rows.length)).toEqual([2, 2, 1, 2]);
    expect(shownSections(table, true).map((s) => s.rows.length)).toEqual([3, 2, 1, 3]);
    expect(shownSections({ ...table, sections: [table.sections[0]!].map((s) => ({ ...s, rows: [s.rows[1]!] })) }, false)).toEqual([]);
    expect(summaryWords(table)).toBe("7 of 9 facts differ between the machines.");
    expect(summaryWords({ ...table, sections: [{ id: "agents", rows: [row(0, 1)] }] })).toBe("Every machine is set up alike: 1 fact compared.");
    expect(summaryWords({ ...table, machines: [table.machines[0]!, { id: "gog", label: "GOG", error: "timeout" }] })).toBe("Connect a second machine to compare how they are set up.");
  });
  it("offers each update remuda has once per machine, with every row it brings in line", () => {
    expect(bringInLine(table).map((b) => [b.machine, b.action.what, b.rows])).toEqual([
      ["gog", "claude", ["Claude Code"]],
      ["gog", "mcp", ["Tasks for Claude Code (MCP)", "Tasks for Codex (MCP)"]],
    ]);
  });
  it("says what is left to do by hand, the newest copy of a file named", () => {
    const left = byHand(table, name, now);
    expect(left.map((h) => h.title)).toEqual(["Codex", "~/.claude/CLAUDE.md", "owner/app · Behind its upstream", "owner/app · CLAUDE.md"]);
    expect(left[0]!.words).toBe("Install Codex on Pi with its own installer, then run remuda setup there for herdr's hooks — or leave it, if it is not wanted there.");
    expect(left[1]!.words).toBe("~/.claude/CLAUDE.md differs on GOG (the copy changed last is GOG's, 2 h ago): copy the one you want to the others by hand. remuda never writes the agents' configs.");
    expect(left[2]!.words).toBe("Pull on GOG: the checkout there is behind its upstream as of its last fetch.");
    expect(left[3]!.words).toBe("CLAUDE.md differs on This Mac, GOG (the copy changed last is This Mac's, 1 h ago): commit it where it changed and pull it on the others.");
    expect(handWords({ id: "claude.model", kind: "model", state: "odd", cells: { gog: { value: "default", mark: "odd" } } }, ["GOG"], ["This Mac"], false)).toContain("/model in Claude Code there");
    expect(handWords({ id: "agent.version", arg: "claude", kind: "version", state: "odd", cells: { gog: { value: "2.1.1", mark: "odd" } } }, ["GOG"], [], false)).toBe("Update Claude Code on GOG by hand: remuda has no update for it there.");
    expect(handWords({ id: "herdr.plugin", arg: "p", kind: "plugin", state: "odd", cells: {} }, ["GOG"], ["Pi"], true)).toBe("Install the plugin on GOG as it is on Pi, or remove it where it is not wanted.");
    const ahead: FleetRow = { id: "agent.version", arg: "claude", kind: "version", state: "odd", norm: "2.1.1", cells: { gog: { value: "2.1.9", mark: "odd" }, pi: { value: "2.1.1", action: update("claude") } } };
    expect(byHand({ ...table, sections: [{ id: "agents", rows: [ahead] }] }, name, now)).toEqual([]);
    setLang("ru");
    expect(byHand(table, name, now)[2]!.words).toBe("Сделайте pull на GOG: копия там отстаёт от upstream на момент последнего fetch.");
  });
});
