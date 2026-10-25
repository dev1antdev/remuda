import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session } from "@/features/roster/api";
import { DEFAULT_RADAR } from "@/features/roster/radar";
import { badgeOf, pressing, sameBadge, titleFor, toneOf } from "./badge";

const NOW = 1_700_000_000_000;

function pane(id: string, agent: string, radar?: Pane["radar"]): Pane {
  return { id, title: id, agent, cwd: `/w/${id}`, focused: false, status: agent ? "idle" : "shell", cols: 80, rows: 24, radar };
}

function session(id: string, panes: Pane[]): Session {
  return { id, name: id, status: "idle", active_tab_id: `${id}-t`, tabs: [{ id: `${id}-t`, label: "1", number: 1, named: false, status: "idle", panes }] };
}

function machine(id: string, panes: Pane[], state: Machine["state"] = "connected"): Machine {
  return { id, label: id, state, sessions: [session(`${id}-s`, panes)] } as unknown as Machine;
}

describe("titleFor", () => {
  it("counts in front only while someone needs the person", () => {
    expect(titleFor(0)).toBe("remuda");
    expect(titleFor(1)).toBe("(1) remuda");
    expect(titleFor(12)).toBe("(12) remuda");
  });
});

describe("toneOf", () => {
  it("is red for a question, amber for a note, green for a finish, nothing otherwise", () => {
    expect(toneOf(pane("a", "claude", { state: "blocked" }), DEFAULT_RADAR, NOW)).toBe("red");
    expect(toneOf(pane("a", "claude", { state: "working", alert: "stuck" }), DEFAULT_RADAR, NOW)).toBe("amber");
    expect(toneOf(pane("a", "claude", { state: "done" }), DEFAULT_RADAR, NOW)).toBe("green");
    expect(toneOf(pane("a", "claude", { state: "working" }), DEFAULT_RADAR, NOW)).toBeNull();
    expect(toneOf(pane("a", "claude", { state: "idle", last_active: NOW - 60_000 }), DEFAULT_RADAR, NOW)).toBeNull();
  });
  it("a question wins over a note on the same agent; a shell counts for nothing", () => {
    expect(toneOf(pane("a", "claude", { state: "blocked", alert: "waiting" }), DEFAULT_RADAR, NOW)).toBe("red");
    expect(toneOf(pane("sh", "", { state: "blocked" }), DEFAULT_RADAR, NOW)).toBeNull();
  });
});

describe("pressing", () => {
  it("keeps the more urgent tone", () => {
    expect(pressing(null, "green")).toBe("green");
    expect(pressing("green", null)).toBe("green");
    expect(pressing("green", "amber")).toBe("amber");
    expect(pressing("red", "amber")).toBe("red");
    expect(pressing("amber", "red")).toBe("red");
    expect(pressing(null, null)).toBeNull();
  });
});

describe("badgeOf", () => {
  it("counts every agent that needs the person over connected machines and takes the most pressing tone", () => {
    const machines = [
      machine("local", [pane("a", "claude", { state: "done" }), pane("b", "codex", { state: "working" }), pane("c", "claude", { state: "working", alert: "loop" })]),
      machine("gog", [pane("d", "claude", { state: "blocked" })]),
    ];
    expect(badgeOf(machines, DEFAULT_RADAR, NOW)).toEqual({ count: 3, tone: "red" });
  });
  it("leaves out a machine that is not connected and reads zero with no tone when nobody needs anyone", () => {
    expect(badgeOf([machine("gog", [pane("d", "claude", { state: "blocked" })], "error")], DEFAULT_RADAR, NOW)).toEqual({ count: 0, tone: null });
    expect(badgeOf([machine("local", [pane("b", "codex", { state: "working" })])], DEFAULT_RADAR, NOW)).toEqual({ count: 0, tone: null });
  });
});

describe("sameBadge", () => {
  it("compares the count and the tone", () => {
    expect(sameBadge({ count: 1, tone: "red" }, { count: 1, tone: "red" })).toBe(true);
    expect(sameBadge({ count: 1, tone: "red" }, { count: 2, tone: "red" })).toBe(false);
    expect(sameBadge({ count: 1, tone: "red" }, { count: 1, tone: "amber" })).toBe(false);
  });
});
