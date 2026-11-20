import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session } from "./api";
import { needsMe, nextNeedingMe } from "./needs";
import { DEFAULT_RADAR } from "./radar";

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
const shell = pane("s", "");
const noted = pane("a", "claude", { state: "working", last_active: NOW, alert: "stuck for 20 min" });

describe("needsMe", () => {
  it("is a question, a watchdog note or a finish not looked at", () => {
    expect(needsMe(blocked, DEFAULT_RADAR, NOW)).toBe(true);
    expect(needsMe(done, DEFAULT_RADAR, NOW)).toBe(true);
    expect(needsMe(noted, DEFAULT_RADAR, NOW)).toBe(true);
    expect(needsMe(working, DEFAULT_RADAR, NOW)).toBe(false);
    expect(needsMe(fresh, DEFAULT_RADAR, NOW)).toBe(false);
    expect(needsMe(shell, DEFAULT_RADAR, NOW)).toBe(false);
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

  it("leaves out a machine that is not connected and the shells", () => {
    expect(nextNeedingMe([machine("gog", [session("b", [blocked])], "connecting"), machine("local", [session("a", [shell, done])])], DEFAULT_RADAR, NOW, "")?.key).toBe("local|d");
  });

  it("answers nobody when every agent is fine, and the same one when it is alone", () => {
    expect(nextNeedingMe([machine("local", [session("a", [working, fresh])])], DEFAULT_RADAR, NOW, "")).toBeNull();
    expect(nextNeedingMe([machine("local", [session("a", [working, blocked])])], DEFAULT_RADAR, NOW, "local|b")?.key).toBe("local|b");
  });
});
