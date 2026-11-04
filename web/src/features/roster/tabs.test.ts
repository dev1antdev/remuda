import { describe, expect, it } from "vitest";
import type { Pane, Tab } from "./api";
import { tabAgent, tabFaces } from "./tabs";

const pane = (over: Partial<Pane> = {}): Pane => ({ id: "p", title: "", agent: "claude", cwd: "/x", focused: false, status: "idle", cols: 80, rows: 24, ...over }) as Pane;
const tab = (id: string, number: number, panes: Pane[], over: Partial<Tab> = {}): Tab => ({ id, number, label: "", named: false, status: "idle", panes, ...over }) as unknown as Tab;

describe("tabAgent", () => {
  it("is the focused agent pane's kind, else the first agent's, none for shells", () => {
    expect(tabAgent(tab("a", 1, [pane({ agent: "" }), pane({ agent: "codex" }), pane({ agent: "claude", focused: true })]))).toBe("claude");
    expect(tabAgent(tab("a", 1, [pane({ agent: "", focused: true }), pane({ agent: "codex" })]))).toBe("codex");
    expect(tabAgent(tab("a", 1, [pane({ agent: "" })]))).toBe("");
  });
});

describe("tabFaces", () => {
  it("names a tab by its agent or as a terminal, keeps a name of the person's own and tells twins apart by number", () => {
    const faces = tabFaces([
      tab("t1", 1, [pane()]),
      tab("t2", 2, [pane({ agent: "" })]),
      tab("t3", 3, [pane({ agent: "" })], { named: true, label: "kubectl" }),
      tab("t4", 4, [pane({ agent: "codex" })]),
    ]);
    expect([...faces.values()]).toEqual([
      { label: "Claude Code", agent: "claude" },
      { label: "terminal", agent: "" },
      { label: "kubectl", agent: "" },
      { label: "Codex", agent: "codex" },
    ]);
    const twins = tabFaces([tab("t1", 1, [pane()]), tab("t2", 2, [pane({ agent: "" })]), tab("t5", 5, [pane()]), tab("t6", 6, [pane({ agent: "" })])]);
    expect([...twins.values()].map((f) => f.label)).toEqual(["Claude Code 1", "terminal 2", "Claude Code 5", "terminal 6"]);
  });

  it("leaves out the names the person gave while masked, telling the twins apart again", () => {
    const masked = tabFaces([tab("t1", 1, [pane()], { named: true, label: "acme-billing" }), tab("t3", 3, [pane({ agent: "" })], { named: true, label: "kubectl prod" }), tab("t4", 4, [pane()])], true);
    expect([...masked.values()].map((f) => f.label)).toEqual(["Claude Code 1", "terminal", "Claude Code 4"]);
  });
});
