import { describe, expect, it } from "vitest";
import type { Standup } from "@/features/standup/api";
import { GAZE_MAX, TUNES, gaze, happenings, joinsApplause, questionBubble, tickerFacts, voiceOf, workedWords, type Seen } from "./zooLife";

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

describe("questionBubble", () => {
  it("holds up the dialog's head and the first line of its detail", () => {
    expect(questionBubble({ text: "Bash command\nrm -rf build\nclean the build\nDo you want to proceed?", options: [] })).toBe("Bash command · rm -rf build");
  });

  it("holds the asking line when the dialog has no head, nothing without a dialog", () => {
    expect(questionBubble({ text: "Which database?", options: [] })).toBe("Which database?");
    expect(questionBubble(null)).toBe("");
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
