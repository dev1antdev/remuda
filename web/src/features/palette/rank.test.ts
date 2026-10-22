import { describe, expect, it } from "vitest";
import type { Machine } from "@/features/roster/api";
import { DEFAULT_RADAR } from "@/features/roster/radar";
import { paneEntries, rank, type Entry } from "./rank";

const entry = (title: string, keywords: string[] = []): Entry => ({ id: title, kind: "command", title, keywords, run: () => {} });
const titles = (entries: Entry[]) => entries.map((e) => e.title);

describe("rank", () => {
  const all = [entry("Go to the Wall", ["mode", "wall"]), entry("Show the terminal", ["lens"]), entry("New workspace on local…", ["start", "agent"]), entry("Wallpaper", ["mode"])];

  it("keeps every entry in its order when nothing is typed", () => {
    expect(rank(all, "  ")).toBe(all);
  });
  it("puts the start of a title before the start of a word in it, and that before its initials", () => {
    expect(titles(rank(all, "wall"))).toEqual(["Wallpaper", "Go to the Wall"]);
    expect(titles(rank(all, "gw"))).toEqual(["Go to the Wall"]);
    expect(titles(rank(all, "gtw"))).toEqual(["Go to the Wall"]);
  });
  it("hits a keyword, but a title hit comes first", () => {
    expect(titles(rank(all, "mode"))).toEqual(["Go to the Wall", "Wallpaper"]);
    expect(titles(rank([entry("Alerts", ["agent"]), entry("New agent", [])], "agent"))).toEqual(["New agent", "Alerts"]);
  });
  it("needs every word to hit, in any order", () => {
    expect(titles(rank(all, "wall go"))).toEqual(["Go to the Wall"]);
    expect(titles(rank(all, "wall nowhere"))).toEqual([]);
  });
  it("takes a regex character as typed", () => {
    expect(titles(rank([entry("a (b)")], "(b"))).toEqual(["a (b)"]);
  });
});

describe("paneEntries", () => {
  const now = 1_000_000_000;
  const pane = (id: string, agent: string, radar?: { state: "working" | "idle" | "blocked"; last_active?: number; alert?: string }, cwd = `/home/me/${id}`) => ({ id, title: "", agent, cwd, focused: false, status: "idle" as const, cols: 80, rows: 24, radar });
  const tab = (id: string, panes: ReturnType<typeof pane>[], label = "") => ({ id, label, number: 1, named: !!label, status: "idle" as const, panes });
  const machines = [
    {
      id: "local",
      label: "mac",
      state: "connected",
      sessions: [
        { id: "w1", name: "remuda", status: "idle", active_tab_id: "t1", tabs: [tab("t1", [pane("p1", "claude", { state: "idle", last_active: now - 60_000 }), pane("p2", "", undefined, "/home/me/site")]), tab("t2", [pane("p3", "codex", { state: "working" })], "deploy")] },
      ],
    },
    { id: "gog", label: "gog", state: "connected", sessions: [{ id: "w2", name: "site", status: "idle", active_tab_id: "t3", tabs: [tab("t3", [pane("p4", "claude", { state: "blocked", alert: "stuck" })])] }] },
  ] as unknown as Machine[];

  it("lists each machine's liveliest first, shells last, with where and what", () => {
    const entries = paneEntries(machines, DEFAULT_RADAR, now, 0, null, () => {});
    expect(entries.map((e) => [e.title, e.hint, e.detail])).toEqual([
      ["Codex", "remuda", "working"],
      ["Claude Code", "remuda", "idle, just stopped · last turn 1 min ago"],
      ["site", "remuda", "shell"],
      ["Claude Code", "site · gog", "stuck"],
    ]);
    expect(entries.map((e) => e.pane!.mark)).toEqual(["⣷", "◦", "$", "!"]);
  });
  it("marks the current pane and runs the open with its ids", () => {
    const opened: string[] = [];
    const entries = paneEntries(machines, DEFAULT_RADAR, now, 0, { machineId: "gog", paneId: "p4" }, (m, p) => opened.push(`${m}/${p}`));
    expect(entries.map((e) => e.pane!.current)).toEqual([false, false, false, true]);
    entries[0]!.run();
    expect(opened).toEqual(["local/p3"]);
  });
  it("is found by its workspace, machine, agent, folder or tab name; an agent before a shell named like its folder", () => {
    const entries = paneEntries(machines, DEFAULT_RADAR, now, 0, null, () => {});
    expect(rank(entries, "gog").map((e) => e.id)).toEqual(["pane:gog|p4"]);
    expect(rank(entries, "deploy").map((e) => e.id)).toEqual(["pane:local|p3"]);
    expect(rank(entries, "site").map((e) => e.id)).toEqual(["pane:gog|p4", "pane:local|p2"]);
  });
});
