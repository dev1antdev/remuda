import { describe, expect, it } from "vitest";
import type { Standup } from "@/features/standup/api";
import { DAY_EVERY, GAZE_MAX, PUFF_MS, TUNES, carried, dayDue, gaze, happenings, joinsApplause, tickerFacts, voiceOf, withPuffs, workedWords, type Seen } from "./zooLife";

const seen = (entries: [string, Seen][]) => new Map(entries);

describe("happenings", () => {
  it("hears a finish, a question, a merge and a compaction between two readings", () => {
    const before = seen([
      ["a", { display: "working", pr: "" }],
      ["b", { display: "idle", pr: "" }],
      ["c", { display: "idle", pr: "OPEN" }],
      ["d", { display: "working", pr: "", usage: { context: 170_000, window: 200_000 } }],
    ]);
    const after = seen([
      ["a", { display: "done", pr: "" }],
      ["b", { display: "blocked", pr: "" }],
      ["c", { display: "idle", pr: "MERGED" }],
      ["d", { display: "working", pr: "", usage: { context: 20_000, window: 200_000 } }],
    ]);
    expect(happenings(before, after)).toEqual({ finished: ["a"], asked: ["b"], merged: ["c"], compacted: ["d"] });
  });

  it("seeds a creature it sees for the first time, so a reload replays nothing", () => {
    expect(happenings(new Map(), seen([["a", { display: "done", pr: "MERGED" }]]))).toEqual({ finished: [], asked: [], merged: [], compacted: [] });
  });

  it("takes a question asked again for no new one", () => {
    const blocked = seen([["b", { display: "blocked", pr: "" }]]);
    expect(happenings(blocked, blocked).asked).toEqual([]);
  });

  it("sets off no fireworks for an old merge when remuda restarts and reads the request again", () => {
    const url = "https://github.com/o/r/pull/15";
    const merged = seen([["c", { display: "idle", pr: "MERGED", url }]]);
    const unread = carried(merged, seen([["c", { display: "idle", pr: "", url }]]));
    expect(unread.get("c")?.pr).toBe("MERGED");
    expect(happenings(unread, carried(unread, seen([["c", { display: "idle", pr: "MERGED", url }]]))).merged).toEqual([]);

    // the link itself gone a moment, or a page that never knew the state: a merge read later is none it saw happen
    const nameless = carried(merged, seen([["c", { display: "idle", pr: "" }]]));
    const named = carried(nameless, seen([["c", { display: "idle", pr: "", url }]]));
    expect(happenings(named, carried(named, seen([["c", { display: "idle", pr: "MERGED", url }]]))).merged).toEqual([]);

    // a request seen open and then merged, whatever came between
    const open = seen([["c", { display: "working", pr: "OPEN", url }]]);
    const between = carried(open, seen([["c", { display: "working", pr: "", url }]]));
    expect(happenings(between, carried(between, seen([["c", { display: "idle", pr: "MERGED", url }]]))).merged).toEqual(["c"]);
  });
});

describe("joinsApplause", () => {
  it("wakes the resting and the puzzled, leaves the busy and the troubled to theirs", () => {
    expect(joinsApplause("asleep")).toBe(true);
    expect(joinsApplause("puzzled")).toBe(true);
    expect(["busy", "asking", "worried", "drained", "dancing"].some((pose) => joinsApplause(pose as never))).toBe(false);
  });
});

describe("gaze", () => {
  it("looks towards the pointer, the full way from far off", () => {
    expect(gaze({ x: 0, y: 0 }, { x: 1000, y: 0 })).toEqual({ x: GAZE_MAX, y: 0 });
    const near = gaze({ x: 0, y: 0 }, { x: 0, y: -90 });
    expect(near.x).toBe(0);
    expect(near.y).toBeLessThan(0);
    expect(Math.abs(near.y)).toBeLessThan(GAZE_MAX);
    expect(gaze({ x: 5, y: 5 }, { x: 5, y: 5 })).toEqual({ x: 0, y: 0 });
  });
});

describe("tickerFacts", () => {
  const item = (pane: string, over: Partial<Standup["items"][number]["recap"]> = {}, state: Standup["items"][number]["state"] = "done") => ({
    machine: "gog",
    label: "GOG",
    workspace: "api",
    pane,
    dir: "/x",
    agent: "claude",
    state,
    recap: { prompts: 1, answers: 2, took: 600, tokens: 0, cost: 1.5, calls: 0, tools: {}, failed: 0, commands: [], edited: [], read: [], subagents: 0, questions: 0, asking: false, ...over },
  });
  const standup: Standup = { since: "", markdown: "", items: [item("p1"), item("p2", { took: 7800, cost: 2.9, answers: 5 }, "blocked")] };

  it("tells the day: answers, waits, spend, merges and who worked longest", () => {
    expect(tickerFacts(standup, 2, (_, pane) => (pane === "p2" ? "web" : "api"))).toEqual([
      "Today: 7 answers",
      "1 waits for you",
      "$4.40 spent",
      "2 pull requests merged",
      "worked longest: web, 2 h 10 min",
    ]);
  });

  it("names no creature the zoo does not show, and says nothing of a quiet day", () => {
    expect(tickerFacts(standup, 0, () => null).some((fact) => fact.startsWith("worked longest"))).toBe(false);
    expect(tickerFacts(null, 0, () => "x")).toEqual([]);
  });

  it("says a stretch of work in hours and minutes", () => {
    expect(workedWords(2520)).toBe("42 min");
    expect(workedWords(7200)).toBe("2 h");
  });
});

describe("voiceOf", () => {
  it("gives every species a voice of its own", () => {
    const voices = (["claude", "codex", "gemini", "kimi", "other"] as const).map(voiceOf).map((v) => `${v.wave}@${v.base}`);
    expect(new Set(voices).size).toBe(voices.length);
    expect(TUNES.merge.length).toBeGreaterThan(TUNES.finish.length);
  });
});

describe("dayDue", () => {
  it("reads the day at once, then not again for minutes: the standup reads every agent's whole conversation", () => {
    expect(DAY_EVERY).toBeGreaterThanOrEqual(5 * 60_000);
    expect(dayDue(0, 1_000)).toBe(true);
    expect(dayDue(1_000, 1_000 + 60_000)).toBe(false);
    expect(dayDue(1_000, 1_000 + DAY_EVERY)).toBe(true);
  });
});

describe("withPuffs", () => {
  it("adds a puff for each compaction and lets the ones over go, so a zoo on all week holds only the puffs on", () => {
    const first = withPuffs(new Map(), ["a"], 1_000);
    expect([...first]).toEqual([["a", 1_000 + PUFF_MS]]);
    const later = withPuffs(first, ["b"], 1_000 + PUFF_MS + 1);
    expect([...later.keys()]).toEqual(["b"]);
    expect(withPuffs(later, ["b"], 1_000 + PUFF_MS + 2).get("b")).toBe(1_000 + 2 * PUFF_MS + 2);
  });
});
