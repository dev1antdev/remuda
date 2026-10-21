import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session } from "./api";
import { DEFAULT_RADAR } from "./radar";
import { DEFAULT_WALL, isIdle, tileKey, tiles } from "./wall";

const NOW = 1_700_000_000_000;
const MIN = 60_000;

function pane(id: string, agent: string, radar?: Pane["radar"]): Pane {
  return { id, title: id, agent, cwd: `/w/${id}`, focused: false, status: agent ? "idle" : "shell", cols: 80, rows: 24, radar };
}

function session(id: string, panes: Pane[]): Session {
  return { id, name: id, status: "idle", active_tab_id: `${id}-t`, tabs: [{ id: `${id}-t`, label: "1", number: 1, named: false, status: "idle", panes }] };
}

function machine(id: string, sessions: Session[], state: Machine["state"] = "connected"): Machine {
  return { id, label: id, state, sessions };
}

const working = pane("w", "claude", { state: "working", last_active: NOW });
const blocked = pane("b", "codex", { state: "blocked", last_active: NOW - 3 * MIN });
const done = pane("d", "claude", { state: "done", last_active: NOW - 40 * MIN });
const fresh = pane("f", "gemini", { state: "idle", last_active: NOW - 5 * MIN });
const old = pane("o", "claude", { state: "idle", last_active: NOW - 3 * 60 * MIN });
const noStamp = pane("n", "claude", { state: "idle" });
const shell = pane("s", "");

describe("tiles", () => {
  it("keeps the roster's order whatever the agents are doing, and skips the shells", () => {
    const list = tiles([machine("local", [session("a", [old, shell, working])]), machine("gog", [session("b", [blocked])])], [], DEFAULT_WALL, DEFAULT_RADAR, NOW);
    expect(list.shown.map((t) => t.key)).toEqual(["local|o", "local|w", "gog|b"]);
    expect(list.shown[0]?.session.name).toBe("a");
    expect(list.hidden).toBe(0);
    expect(list.idle).toBe(0);
  });

  it("leaves out a machine that is not connected", () => {
    const list = tiles([machine("gog", [session("b", [working])], "connecting"), machine("roofr", [session("c", [blocked])], "error")], [], DEFAULT_WALL, DEFAULT_RADAR, NOW);
    expect(list.shown).toEqual([]);
  });

  it("drops the tiles hidden by hand and counts them", () => {
    const list = tiles([machine("local", [session("a", [working, blocked, done])])], [tileKey("local", "b"), tileKey("local", "gone")], DEFAULT_WALL, DEFAULT_RADAR, NOW);
    expect(list.shown.map((t) => t.pane.id)).toEqual(["w", "d"]);
    expect(list.hidden).toBe(1);
  });

  it("hides the idle agents past the fresh tier when asked, never a working, asking, finished or just-stopped one", () => {
    const all = [working, blocked, done, fresh, old, noStamp];
    const prefs = { ...DEFAULT_WALL, hideIdle: true };
    const list = tiles([machine("local", [session("a", all)])], [], prefs, DEFAULT_RADAR, NOW);
    expect(list.shown.map((t) => t.pane.id)).toEqual(["w", "b", "d", "f"]);
    expect(list.idle).toBe(2);
    expect(tiles([machine("local", [session("a", all)])], [], DEFAULT_WALL, DEFAULT_RADAR, NOW).shown).toHaveLength(6);
  });
});

describe("isIdle", () => {
  it("follows the radar's tiers", () => {
    expect(isIdle(fresh, DEFAULT_RADAR, NOW)).toBe(false);
    expect(isIdle(old, DEFAULT_RADAR, NOW)).toBe(true);
    expect(isIdle(noStamp, DEFAULT_RADAR, NOW)).toBe(true);
    expect(isIdle(done, DEFAULT_RADAR, NOW)).toBe(false);
    expect(isIdle(fresh, { ...DEFAULT_RADAR, freshMinutes: 2 }, NOW)).toBe(true);
  });
});
