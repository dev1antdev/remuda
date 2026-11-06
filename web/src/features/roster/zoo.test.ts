import { describe, expect, it } from "vitest";
import type { Pane } from "./api";
import { BUBBLE_MAX, CREATURE_SIZE, bubbleOf, cellMin, lookOf, poseWord, stateSince, zooFace, type Pose } from "./zoo";

const pane = (over: Partial<Pane> = {}): Pane => ({ id: "p", title: "", agent: "claude", cwd: "/x", focused: false, status: "idle", cols: 80, rows: 24, ...over }) as Pane;

describe("lookOf", () => {
  it("gives every state a pose and a sign of its own, so it reads without colour", () => {
    expect(lookOf("working")).toEqual({ pose: "busy", sign: "", dim: false });
    expect(lookOf("blocked")).toEqual({ pose: "asking", sign: "?", dim: false });
    expect(lookOf("done")).toEqual({ pose: "dancing", sign: "", dim: false });
    expect(lookOf("idle")).toEqual({ pose: "asleep", sign: "z", dim: false });
    expect(lookOf("idle_fresh")).toEqual({ pose: "asleep", sign: "z", dim: false });
    expect(lookOf("idle_stale")).toEqual({ pose: "asleep", sign: "z", dim: true });
    expect(lookOf("unknown")).toEqual({ pose: "puzzled", sign: "…", dim: false });
  });

  it("puts a watchdog note above every other state", () => {
    expect(lookOf("working", "stuck for 20 min")).toEqual({ pose: "worried", sign: "!", dim: false });
    expect(lookOf("idle_stale", "the same call six times").sign).toBe("!");
  });
});

describe("bubbleOf", () => {
  it("holds the answer's first line once the agent finished or rests", () => {
    const answered = pane({ said: "Fixed the queue: it drains again" });
    expect(bubbleOf(answered, "done")).toBe("Fixed the queue: it drains again");
    expect(bubbleOf(answered, "idle_stale")).toBe("Fixed the queue: it drains again");
    expect(bubbleOf(answered, "working")).toBe("");
    expect(bubbleOf(answered, "blocked")).toBe("");
    expect(bubbleOf(answered, "done", "a note")).toBe("");
    expect(bubbleOf(pane(), "done")).toBe("");
  });

  it("never cuts an answer between the halves of an emoji", () => {
    const bubble = bubbleOf(pane({ said: `a${"🚀".repeat(60)}` }), "done");
    expect(bubble).toBe(`a${"🚀".repeat(44)}…`);
  });

  it("cuts a long answer at a word", () => {
    const long = pane({ said: "word ".repeat(40).trim() });
    const bubble = bubbleOf(long, "done");
    expect(bubble.length).toBeLessThanOrEqual(BUBBLE_MAX + 1);
    expect(bubble.endsWith("word…")).toBe(true);
    const unbroken = pane({ said: "x".repeat(200) });
    expect(bubbleOf(unbroken, "done")).toBe(`${"x".repeat(BUBBLE_MAX)}…`);
  });
});

describe("poseWord", () => {
  it("names every pose in a word of its own", () => {
    const poses: Pose[] = ["busy", "asking", "worried", "dancing", "asleep", "puzzled"];
    const words = poses.map(poseWord);
    expect(new Set(words).size).toBe(poses.length);
    expect(poseWord("asking")).toBe("asking you");
  });
});

describe("the creatures' sizes", () => {
  it("grow with the wall's size, the cell with them", () => {
    expect(CREATURE_SIZE.s).toBeLessThan(CREATURE_SIZE.m);
    expect(CREATURE_SIZE.m).toBeLessThan(CREATURE_SIZE.l);
    expect(cellMin("m")).toBeGreaterThan(CREATURE_SIZE.m);
  });
});

describe("stateSince", () => {
  it("counts work and questions from their start, the rest from the last turn", () => {
    expect(stateSince(pane({ radar: { state: "working", since: 5, last_active: 9 } }))).toBe(5);
    expect(stateSince(pane({ radar: { state: "blocked", since: 7 } }))).toBe(7);
    expect(stateSince(pane({ radar: { state: "done", since: 5, last_active: 9 } }))).toBe(9);
    expect(stateSince(pane({ radar: { state: "idle" } }))).toBeUndefined();
    expect(stateSince(pane())).toBeUndefined();
  });
});

describe("zooFace", () => {
  const face = { label: "api · fix the login", kind: "Claude Code", folder: "api", machine: "GOG", bubble: "Fixed the login form" };

  it("says the label, the folder, the machine and the bubble", () => {
    expect(zooFace(face, false)).toEqual({ name: "api · fix the login", where: "api · GOG", bubble: "Fixed the login form", icon: true });
    expect(zooFace({ ...face, machine: null }, false).where).toBe("api");
  });

  it("keeps the agent's kind and the neutral names alone while the folder is hidden", () => {
    const hidden = zooFace({ ...face, folder: "Project A", machine: "Machine 1" }, true);
    expect(hidden).toEqual({ name: "Claude Code", where: "Project A · Machine 1", bubble: "", icon: false });
    expect(JSON.stringify(hidden)).not.toMatch(/login|api|GOG/);
  });
});
