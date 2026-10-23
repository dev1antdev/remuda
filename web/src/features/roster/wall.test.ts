import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session } from "./api";
import { DEFAULT_RADAR } from "./radar";
import { DEFAULT_WALL, isIdle, needsMe, nextNeedingMe, tileKey, tiles, withFocus } from "./wall";

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
const noted = pane("a", "claude", { state: "working", last_active: NOW, alert: "stuck for 20 min" });

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

describe("needs me", () => {
  it("keeps only the agents waiting for the person — a question, a watchdog note, a finish not looked at — and counts the rest", () => {
    const all = [working, blocked, done, fresh, old, noStamp, noted];
    const list = tiles([machine("local", [session("a", all)])], [], { ...DEFAULT_WALL, needsMe: true }, DEFAULT_RADAR, NOW);
    expect(list.shown.map((t) => t.pane.id)).toEqual(["b", "d", "a"]);
    expect(list.calm).toBe(4);
    expect(list.idle).toBe(0);
    const both = tiles([machine("local", [session("a", all)])], [], { ...DEFAULT_WALL, needsMe: true, hideIdle: true }, DEFAULT_RADAR, NOW);
    expect(both.shown.map((t) => t.pane.id)).toEqual(["b", "d", "a"]);
    expect(both.calm).toBe(4);
  });

  it("is a question, a note or an unseen finish, never work or rest", () => {
    expect(needsMe(blocked, DEFAULT_RADAR, NOW)).toBe(true);
    expect(needsMe(done, DEFAULT_RADAR, NOW)).toBe(true);
    expect(needsMe(noted, DEFAULT_RADAR, NOW)).toBe(true);
    expect(needsMe(working, DEFAULT_RADAR, NOW)).toBe(false);
    expect(needsMe(fresh, DEFAULT_RADAR, NOW)).toBe(false);
    expect(needsMe(shell, DEFAULT_RADAR, NOW)).toBe(false);
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

describe("withFocus", () => {
  it("lifts the focused tile out and keeps the rest in order; a gone key focuses nothing", () => {
    const shown = tiles([machine("local", [session("a", [working, blocked, done])])], [], DEFAULT_WALL, DEFAULT_RADAR, NOW).shown;
    const focused = withFocus(shown, tileKey("local", "b"));
    expect(focused.big?.pane.id).toBe("b");
    expect(focused.rest.map((t) => t.pane.id)).toEqual(["w", "d"]);
    const none = withFocus(shown, tileKey("local", "gone"));
    expect(none.big).toBeNull();
    expect(none.rest).toBe(shown);
  });
});

describe("nextNeedingMe", () => {
  const roster = [machine("local", [session("a", [working, blocked, fresh])]), machine("gog", [session("b", [done, noted])])];

  it("walks the agents that need the person in the roster's order, round and round", () => {
    expect(nextNeedingMe(roster, DEFAULT_RADAR, NOW, "")?.key).toBe("local|b");
    expect(nextNeedingMe(roster, DEFAULT_RADAR, NOW, "local|b")?.key).toBe("gog|d");
    expect(nextNeedingMe(roster, DEFAULT_RADAR, NOW, "gog|d")?.key).toBe("gog|a");
    expect(nextNeedingMe(roster, DEFAULT_RADAR, NOW, "gog|a")?.key).toBe("local|b");
  });

  it("starts from the first when the given one needs nobody or is unknown", () => {
    expect(nextNeedingMe(roster, DEFAULT_RADAR, NOW, "local|w")?.key).toBe("local|b");
    expect(nextNeedingMe(roster, DEFAULT_RADAR, NOW, "nowhere|x")?.key).toBe("local|b");
  });

  it("answers nobody when every agent is fine, and the same one when it is alone", () => {
    expect(nextNeedingMe([machine("local", [session("a", [working, fresh])])], DEFAULT_RADAR, NOW, "")).toBeNull();
    expect(nextNeedingMe([machine("local", [session("a", [working, blocked])])], DEFAULT_RADAR, NOW, "local|b")?.key).toBe("local|b");
  });
});
