import { describe, expect, it } from "vitest";
import type { LimitWindow, PullRequest } from "./api";
import { BELT_AT, COFFEE_AFTER, MAX_BABIES, SWEAT_AFTER, babiesOf, broodWords, coinsOf, compacted, drainedUntil, flagOf, flagWords, girthOf, workExtras } from "./zooBody";

const pr = (over: Partial<NonNullable<PullRequest["status"]>> = {}, checks: Partial<NonNullable<PullRequest["status"]>["checks"]> = {}): PullRequest => ({
  url: "https://github.com/o/r/pull/15",
  label: "PR #15",
  status: { state: "OPEN", draft: false, mergeable: "MERGEABLE", review: "", at: "", ...over, checks: { total: 0, pass: 0, fail: 0, pending: 0, skipped: 0, ...checks } },
});

describe("girthOf", () => {
  it("is the share of the window the context holds, kept within 0 and 1", () => {
    expect(girthOf({ context: 50_000, window: 200_000 })).toBe(0.25);
    expect(girthOf({ context: 300_000, window: 200_000 })).toBe(1);
    expect(girthOf({ context: 10, window: 0 })).toBe(0);
    expect(girthOf(undefined)).toBe(0);
    expect(BELT_AT).toBe(0.8);
  });
});

describe("compacted", () => {
  it("hears a full context shrink to under half", () => {
    expect(compacted({ context: 160_000, window: 200_000 }, { context: 30_000, window: 200_000 })).toBe(true);
  });

  it("takes neither a small context's dip nor a slight shrink for a compaction", () => {
    expect(compacted({ context: 20_000, window: 200_000 }, { context: 2_000, window: 200_000 })).toBe(false);
    expect(compacted({ context: 160_000, window: 200_000 }, { context: 120_000, window: 200_000 })).toBe(false);
    expect(compacted(undefined, { context: 1, window: 200_000 })).toBe(false);
  });
});

describe("coinsOf", () => {
  it("piles a coin per step of cost, up to six", () => {
    expect(coinsOf(undefined)).toBe(0);
    expect(coinsOf(0.01)).toBe(0);
    expect(coinsOf(0.05)).toBe(1);
    expect(coinsOf(1.2)).toBe(3);
    expect(coinsOf(12)).toBe(5);
    expect(coinsOf(400)).toBe(6);
  });
});

describe("flagOf", () => {
  it("flies the checks' state, failures first", () => {
    expect(flagOf(pr({}, { total: 3, fail: 1, pending: 1 }))).toBe("fail");
    expect(flagOf(pr({}, { total: 3, pending: 1, pass: 2 }))).toBe("pending");
    expect(flagOf(pr({}, { total: 3, pass: 2, skipped: 1 }))).toBe("pass");
    expect(flagOf(pr({ draft: true }, { total: 2, pass: 2 }))).toBe("draft");
    expect(flagOf(pr())).toBe("open");
  });

  it("crowns a merge and flies nothing for a closed request or one not read", () => {
    expect(flagOf(pr({ state: "MERGED" }, { total: 1, fail: 1 }))).toBe("merged");
    expect(flagOf(pr({ state: "CLOSED" }))).toBe("");
    expect(flagOf({ url: "u", label: "PR #1" })).toBe("");
    expect(flagOf(undefined)).toBe("");
  });

  it("says the flag in words", () => {
    expect(flagWords("merged", "PR #15")).toBe("PR #15 merged");
    expect(flagWords("fail", "PR #15")).toBe("PR #15: checks failed");
    expect(flagWords("", "PR #15")).toBe("");
  });
});

describe("workExtras", () => {
  const now = 10_000_000;
  it("brings the mug out after 20 minutes of work and the sweat after an hour", () => {
    expect(workExtras("working", now - COFFEE_AFTER + 1, now)).toEqual({ coffee: false, sweat: false });
    expect(workExtras("working", now - COFFEE_AFTER, now)).toEqual({ coffee: true, sweat: false });
    expect(workExtras("working", now - SWEAT_AFTER, now)).toEqual({ coffee: true, sweat: true });
    expect(workExtras("done", now - SWEAT_AFTER, now)).toEqual({ coffee: false, sweat: false });
  });

  it("has everyone in a hot pen sweat", () => {
    expect(workExtras("idle", undefined, now, true)).toEqual({ coffee: false, sweat: true });
  });
});

describe("drainedUntil", () => {
  const windows: LimitWindow[] = [
    { agent: "claude", name: "5h", used: 100, resets_at: "2026-11-12T14:00:00Z", at: "" },
    { agent: "claude", name: "week", used: 100, resets_at: "2026-11-14T09:00:00Z", at: "" },
    { agent: "codex", name: "5h", used: 100, resets_at: "2026-11-12T12:00:00Z", at: "" },
  ];

  it("waits on the latest full window of the agent's own", () => {
    expect(drainedUntil({ hit_at: "x" }, windows, "claude")).toBe("2026-11-14T09:00:00Z");
  });

  it("goes by the person's booking when there is one, and says nothing without a hit", () => {
    expect(drainedUntil({ hit_at: "x", continue_at: "2026-11-12T15:00:00Z" }, windows, "claude")).toBe("2026-11-12T15:00:00Z");
    expect(drainedUntil(undefined, windows, "claude")).toBeUndefined();
  });
});

describe("babiesOf", () => {
  it("stands the working subagents by the creature and counts the rest", () => {
    expect(babiesOf({ working: 2 })).toEqual({ babies: 2, more: 0 });
    expect(babiesOf({ working: 5 })).toEqual({ babies: 3, more: 2 });
    expect(babiesOf(undefined)).toEqual({ babies: 0, more: 0 });
    expect(MAX_BABIES).toBe(3);
  });

  it("has none while no subagent works: the roster counts only the ones at work", () => {
    expect(babiesOf({})).toEqual({ babies: 0, more: 0 });
    expect(broodWords({})).toBe("");
    expect(broodWords({ working: 2 })).toBe("2 subagents working");
  });
});
