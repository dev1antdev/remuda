import { describe, expect, it } from "vitest";
import type { Item, Recap } from "./api";
import { AWAY_AFTER, awayCounts, awayGap, awayLine, awayWords } from "./away";

const NOW = Date.parse("2026-10-25T12:00:00Z");
const H = 3600_000;

function recap(over: Partial<Recap> = {}): Recap {
  return { prompts: 0, answers: 0, took: 0, tokens: 0, cost: 0, calls: 0, tools: {}, failed: 0, commands: [], edited: [], read: [], subagents: 0, questions: 0, asking: false, ...over };
}

function item(state: Item["state"], over: Partial<Recap> = {}, alert?: string): Item {
  return { machine: "local", label: "", workspace: "w", pane: "p", dir: "/w", agent: "claude", state, alert, recap: recap(over) };
}

describe("awayGap", () => {
  it("is the gap from an hour on, nothing under it", () => {
    expect(awayGap(new Date(NOW - 3 * H).toISOString(), NOW)).toBe(3 * H);
    expect(awayGap(new Date(NOW - AWAY_AFTER).toISOString(), NOW)).toBe(AWAY_AFTER);
    expect(awayGap(new Date(NOW - 59 * 60_000).toISOString(), NOW)).toBeNull();
  });
  it("is nothing without a stamp or with one that does not parse", () => {
    expect(awayGap("", NOW)).toBeNull();
    expect(awayGap(null, NOW)).toBeNull();
    expect(awayGap(undefined, NOW)).toBeNull();
    expect(awayGap("yesterday", NOW)).toBeNull();
  });
});

describe("awayWords", () => {
  it("counts hours under a day and days from there", () => {
    expect(awayWords(H)).toBe("1 h");
    expect(awayWords(3.7 * H)).toBe("3 h");
    expect(awayWords(23 * H)).toBe("23 h");
    expect(awayWords(26 * H)).toBe("1 day");
    expect(awayWords(50 * H)).toBe("2 days");
  });
});

describe("awayCounts", () => {
  it("counts the agents that answered, the ones waiting, and the cost", () => {
    const items = [
      item("done", { answers: 2, cost: 1.5 }),
      item("blocked", { answers: 1, cost: 0.25, asking: true }),
      item("working", { answers: 0, cost: 0 }, "stuck"),
      item("idle"),
    ];
    expect(awayCounts(items)).toEqual({ answered: 2, waiting: 2, cost: 1.75 });
  });
});

describe("awayLine", () => {
  it("names what happened and how long the person was away", () => {
    const items = [item("done", { answers: 2, cost: 3.2 }), item("done", { answers: 1, cost: 1 }), item("blocked", { answers: 0, asking: true })];
    expect(awayLine(items, 3 * H)).toBe("Away 3 h: 2 agents answered, 1 waits for you, $4.20 spent");
  });
  it("drops the parts that are zero and reads as nothing when nothing happened", () => {
    expect(awayLine([item("done", { answers: 1 })], 2 * H)).toBe("Away 2 h: 1 agent answered");
    expect(awayLine([item("blocked"), item("blocked")], 30 * H)).toBe("Away 1 day: 2 wait for you");
    expect(awayLine([item("idle"), item("working")], 2 * H)).toBeNull();
    expect(awayLine([], 2 * H)).toBeNull();
  });
});
