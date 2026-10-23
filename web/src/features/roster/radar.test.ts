import { describe, expect, it } from "vitest";
import type { Pane, Tab } from "./api";
import { DEFAULT_RADAR, displayOf, FRAMES, markOf, PULSE_STEPS, rowLabel, stateWords, tier } from "./radar";

const now = 1_000_000_000;
const minutes = (n: number) => now - n * 60_000;
const pane = (over: Partial<Pane>): Pane => ({ id: "p", title: "", agent: "claude", cwd: "/home/me/remuda", focused: false, status: "idle", cols: 80, rows: 24, ...over }) as Pane;
const tab = (over: Partial<Tab>): Tab => ({ id: "t", label: "tab 1", named: false, panes: [] }) as unknown as Tab;

describe("tier", () => {
  it("cools with the time since the last turn; no stamp is plain", () => {
    expect(tier(undefined, DEFAULT_RADAR, now)).toBe("normal");
    expect(tier(minutes(10), DEFAULT_RADAR, now)).toBe("fresh");
    expect(tier(minutes(15), DEFAULT_RADAR, now)).toBe("fresh");
    expect(tier(minutes(60), DEFAULT_RADAR, now)).toBe("normal");
    expect(tier(minutes(120), DEFAULT_RADAR, now)).toBe("stale");
  });
});

describe("displayOf", () => {
  it("passes the server's state through and tiers idle", () => {
    expect(displayOf(pane({}), DEFAULT_RADAR, now)).toBe("idle");
    expect(displayOf(pane({ radar: { state: "working" } }), DEFAULT_RADAR, now)).toBe("working");
    expect(displayOf(pane({ radar: { state: "idle", last_active: minutes(1) } }), DEFAULT_RADAR, now)).toBe("idle_fresh");
    expect(displayOf(pane({ radar: { state: "idle", last_active: minutes(500) } }), DEFAULT_RADAR, now)).toBe("idle_stale");
  });
});

describe("markOf", () => {
  it("spins while working, pulses while blocked, ticks when done", () => {
    expect(markOf("working", 0)).toBe(FRAMES[0]);
    expect(markOf("working", FRAMES.length + 2)).toBe(FRAMES[2]);
    expect(markOf("blocked", 0)).toBe("?");
    expect(markOf("blocked", PULSE_STEPS)).toBe("◦");
    expect(markOf("done", 0)).toBe("✓");
    expect(markOf("idle", 0)).toBe("◦");
  });
});

describe("rowLabel", () => {
  it("falls back from a title that only names the folder, and drops the workspace's prefix", () => {
    expect(rowLabel(pane({ title: "remuda", agent: "zzz-vendor" }), tab({}), "", DEFAULT_RADAR)).toBe("zzz-vendor");
    expect(rowLabel(pane({ title: "remuda: fix the tests" }), tab({}), "remuda", DEFAULT_RADAR)).toBe("fix the tests");
  });
  it("uses the tab's name when asked and the tab was named", () => {
    const named = { ...tab({}), named: true, label: "api" } as Tab;
    expect(rowLabel(pane({ title: "a long title" }), named, "", { ...DEFAULT_RADAR, rowLabel: "tab" })).toBe("api");
    expect(rowLabel(pane({ title: "a long title" }), named, "", { ...DEFAULT_RADAR, rowLabel: "both" })).toBe("api · a long title");
    expect(rowLabel(pane({ title: "a long title" }), tab({}), "", { ...DEFAULT_RADAR, rowLabel: "tab" })).toBe("a long title");
  });
});

describe("stateWords", () => {
  it("says the state, how long it has lasted and when the last turn was", () => {
    const now = 1_700_000_000_000;
    expect(stateWords("working", undefined, now)).toBe("working");
    expect(stateWords("working", undefined, now, now - 6 * 60_000)).toBe("working for 6 min");
    expect(stateWords("blocked", undefined, now, now - 90_000)).toBe("waiting on you for 2 min");
    expect(stateWords("done", now - 4 * 60_000, now, now - 60_000)).toBe("done · last turn 4 min ago");
  });
});
