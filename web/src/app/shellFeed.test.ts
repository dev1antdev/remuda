import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session } from "@/features/roster/api";
import { DEFAULT_RADAR } from "@/features/roster/radar";
import { MAX_ROWS, modeOf, sameFeed, shellRows } from "./shellFeed";

const NOW = 1_700_000_000_000;

function pane(id: string, agent: string, radar?: Pane["radar"], said?: string): Pane {
  return { id, title: `${id} title`, agent, cwd: `/w/${id}`, focused: false, status: agent ? "idle" : "shell", cols: 80, rows: 24, radar, said };
}

function session(id: string, panes: Pane[]): Session {
  return { id, name: id, status: "idle", active_tab_id: `${id}-t`, tabs: [{ id: `${id}-t`, label: "1", number: 1, named: false, status: "idle", panes }] };
}

function machine(id: string, panes: Pane[], state: Machine["state"] = "connected"): Machine {
  return { id, label: `${id} label`, state, sessions: [session(`${id}-s`, panes)] } as unknown as Machine;
}

describe("shellRows", () => {
  it("lists the agents that need the person with their tone and words, and counts the rest", () => {
    const m = machine("local", [
      pane("q", "claude", { state: "blocked" }, "an old answer"),
      pane("n", "claude", { state: "working", alert: "stuck 24 min" }),
      pane("d", "codex", { state: "done" }, "Done: the tests pass"),
      pane("w", "claude", { state: "working" }),
      pane("i", "claude", { state: "idle", last_active: NOW - 60_000 }),
      pane("sh", "", { state: "blocked" }),
    ]);
    const feed = shellRows([m], DEFAULT_RADAR, NOW);
    expect(feed.working).toBe(1);
    expect(feed.idle).toBe(1);
    // the sidebar's order: the tab's first pane leads, then the liveliest
    expect(feed.rows.map((r) => [r.pane, r.tone, r.said, r.state])).toEqual([
      ["q", "red", "waiting for an answer", "blocked"],
      ["n", "amber", "stuck 24 min", "working"],
      ["d", "green", "Done: the tests pass", "done"],
    ]);
    expect(feed.rows[1]).toMatchObject({ machine: "local", machineLabel: "local label", workspace: "local-s", title: "n title" });
  });
  it("skips machines that are not connected and caps the rows", () => {
    const many = Array.from({ length: MAX_ROWS + 3 }, (_, i) => pane(`p${i}`, "claude", { state: "blocked" }));
    const feed = shellRows([machine("away", [pane("x", "claude", { state: "blocked" })], "error"), machine("local", many)], DEFAULT_RADAR, NOW);
    expect(feed.rows).toHaveLength(MAX_ROWS);
    expect(feed.rows.every((r) => r.machine === "local")).toBe(true);
  });
  it("is empty on a quiet roster", () => {
    expect(shellRows([machine("local", [pane("w", "claude", { state: "working" })])], DEFAULT_RADAR, NOW)).toEqual({ rows: [], working: 1, idle: 0 });
  });
});

describe("sameFeed", () => {
  it("compares the rows and the counts field by field", () => {
    const a = shellRows([machine("local", [pane("q", "claude", { state: "blocked" })])], DEFAULT_RADAR, NOW);
    const b = shellRows([machine("local", [pane("q", "claude", { state: "blocked" })])], DEFAULT_RADAR, NOW);
    expect(sameFeed(a, b)).toBe(true);
    expect(sameFeed(a, { ...b, working: 1 })).toBe(false);
    expect(sameFeed(a, { ...b, rows: [{ ...b.rows[0]!, said: "other" }] })).toBe(false);
    expect(sameFeed(a, { ...b, rows: [] })).toBe(false);
  });
});

describe("modeOf", () => {
  it("reads the app's names, agents for the first mode", () => {
    expect(modeOf("agents")).toBe("herdr");
    expect(modeOf("wall")).toBe("wall");
    expect(modeOf("code")).toBe("code");
    expect(modeOf("settings")).toBeNull();
    expect(modeOf(3)).toBeNull();
  });
});
