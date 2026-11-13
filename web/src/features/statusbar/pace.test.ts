import { describe, expect, it } from "vitest";
import type { AccountWindow } from "@/features/usage/limits";
import { byAgent, elapsedShare, segmentText, shownShare, stale, tickWords, timeGone, timeLeft, verdict, windowMs } from "./pace";

const now = Date.parse("2026-11-13T12:00:00Z");
const win = (over: Partial<AccountWindow>): AccountWindow => ({ agent: "claude", name: "5h", used: 38, resets_at: new Date(now + 3 * 3600e3).toISOString(), at: new Date(now - 60e3).toISOString(), machine: "local", machineLabel: "Mac", ...over });

describe("the pace of a window", () => {
  it("knows the windows' lengths and how much of their time is gone", () => {
    expect(windowMs("5h")).toBe(5 * 3600e3);
    expect(windowMs("week")).toBe(7 * 86400e3);
    expect(windowMs("month")).toBeNull();
    expect(elapsedShare(win({}), now)).toBeCloseTo(0.4);
    expect(elapsedShare(win({ resets_at: new Date(now - 1).toISOString() }), now)).toBe(1);
    expect(elapsedShare(win({ resets_at: new Date(now + 9 * 3600e3).toISOString() }), now)).toBe(0);
    expect(elapsedShare(win({ name: "month" }), now)).toBeNull();
  });

  it("says the use runs ahead of the time, on it or under it by ten points", () => {
    expect(verdict(62, 0.4)).toBe("ahead");
    expect(verdict(50, 0.4)).toBe("on");
    expect(verdict(30, 0.4)).toBe("on");
    expect(verdict(29, 0.4)).toBe("under");
    expect(tickWords(62, 0.4, false)).toBe("The tick: 40 % of the time has passed. You have used 62 %: you are using it faster than the time passes.");
    expect(timeGone(0.4, false)).toBe("40 % of the time gone");
    expect(timeGone(0.4, true)).toBe("60 % of the time left");
    expect(tickWords(62, 0.4, true)).toBe("The tick: 60 % of the time is left. You have 38 % left: you are using it faster than the time passes.");
  });

  it("writes a window in a few characters, used or remaining, and the time to its reset", () => {
    expect(segmentText(win({ used: 38.9 }), false)).toBe("5h 38%");
    expect(segmentText(win({ name: "week", used: 54.2 }), true)).toBe("wk 46%");
    expect(shownShare(130, false)).toBe(100);
    expect(shownShare(130, true)).toBe(0);
    expect(timeLeft(new Date(now + 30e3).toISOString(), now)).toBe("<1m");
    expect(timeLeft(new Date(now + 42 * 60e3).toISOString(), now)).toBe("42m");
    expect(timeLeft(new Date(now + 130 * 60e3).toISOString(), now)).toBe("2h 10m");
    expect(timeLeft(new Date(now + (3 * 24 + 4) * 3600e3).toISOString(), now)).toBe("3d 4h");
  });

  it("fades a reading past a quarter of an hour", () => {
    expect(stale(new Date(now - 14 * 60e3).toISOString(), now)).toBe(false);
    expect(stale(new Date(now - 16 * 60e3).toISOString(), now)).toBe(true);
  });

  it("groups the windows by agent with the gravest tone and the newest reading", () => {
    const groups = byAgent([win({ used: 82 }), win({ name: "week", used: 40, at: new Date(now).toISOString() }), win({ agent: "codex", name: "week", used: 100 })]);
    expect(groups.map((g) => `${g.agent} ${g.windows.length} ${g.tone}`)).toEqual(["claude 2 amber", "codex 1 red"]);
    expect(groups[0]!.at).toBe(new Date(now).toISOString());
  });
});
