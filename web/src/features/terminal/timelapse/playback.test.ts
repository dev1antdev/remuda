import { describe, expect, it } from "vitest";
import { advance, FRAME_MS, frameAt, gifDelays, HOLD_MS, MIN_DELAY, playedFor, playerKey, screensOf, screenText, stepTo } from "./playback";

const times = [0, 4000, 8000, 60_000, 64_000];

describe("screensOf", () => {
  it("puts every frame back together from the rows that moved", () => {
    const screens = screensOf([
      { at: 0, lines: 3, rows: [{ line: 0, text: "\x1b[1mhead\x1b[0m" }, { line: 1, text: "step 1" }, { line: 2, text: "> " }] },
      { at: 4000, lines: 3, rows: [{ line: 1, text: "step 2" }] },
      { at: 8000, lines: 2, rows: [] },
      { at: 9000, lines: 4, rows: [{ line: 3, text: "tail" }, { line: 9, text: "past the screen" }] },
    ]);
    expect(screens.map(screenText)).toEqual(["\x1b[1mhead\x1b[0m\nstep 1\n> ", "\x1b[1mhead\x1b[0m\nstep 2\n> ", "\x1b[1mhead\x1b[0m\nstep 2", "\x1b[1mhead\x1b[0m\nstep 2\n\ntail"]);
    expect(screenText(undefined)).toBe("");
  });
  it("keeps a row once however many frames show it", () => {
    const wide = "x".repeat(10_000);
    const screens = screensOf(Array.from({ length: 50 }, (_, i) => ({ at: i * 4000, lines: 2, rows: i === 0 ? [{ line: 0, text: wide }, { line: 1, text: "0" }] : [{ line: 1, text: String(i) }] })));
    expect(screens.every((rows) => rows[0] === wide && rows.length === 2)).toBe(true);
    expect(screens.every((rows) => typeof rows !== "string")).toBe(true);
  });
});

describe("frameAt and stepTo", () => {
  it("shows the last frame at or before the moment", () => {
    expect([-1, 0, 3999, 4000, 59_999, 64_000, 99_000].map((t) => frameAt(times, t))).toEqual([0, 0, 0, 1, 2, 4, 4]);
    expect(frameAt([], 5)).toBe(0);
  });
  it("steps a frame either way and stops at the ends", () => {
    expect(stepTo(times, 5000, 1)).toBe(8000);
    expect(stepTo(times, 5000, -1)).toBe(0);
    expect(stepTo(times, 0, -1)).toBe(0);
    expect(stepTo(times, 64_000, 1)).toBe(64_000);
  });
});

describe("advance", () => {
  it("runs the turn's clock at the speed picked and ends at its end", () => {
    expect(advance({ t: 0, carry: 0 }, 100, 30, times, 70_000)).toEqual({ pos: { t: 3000, carry: 0 }, ended: false });
    expect(advance({ t: 69_000, carry: 0 }, 100, 60, times, 70_000)).toEqual({ pos: { t: 70_000, carry: 0 }, ended: true });
  });
  it("by frame, shows each frame for the same time whatever the gap behind it", () => {
    let pos = { t: 8000, carry: 0 };
    ({ pos } = advance(pos, FRAME_MS - 1, "frame", times, 70_000));
    expect(pos.t).toBe(8000);
    ({ pos } = advance(pos, 1, "frame", times, 70_000));
    expect(pos).toEqual({ t: 60_000, carry: 0 });
    const last = advance(pos, 3 * FRAME_MS, "frame", times, 70_000);
    expect(last).toEqual({ pos: { t: 64_000, carry: 0 }, ended: true });
  });
  it("has nothing to play without frames", () => {
    expect(advance({ t: 0, carry: 0 }, 100, 10, [], 0).ended).toBe(true);
  });
});

describe("gifDelays and playedFor", () => {
  it("keeps the turn's own gaps at the speed, rounded on the running total, the last frame held", () => {
    expect(gifDelays(times, 60)).toEqual([7, 6, 87, 7, HOLD_MS / 10]);
    expect(gifDelays([0, 10, 20], 60)).toEqual([MIN_DELAY, MIN_DELAY, HOLD_MS / 10]);
    expect(gifDelays(times, "frame")).toEqual([25, 25, 25, 25, HOLD_MS / 10]);
  });
  it("says how long a play lasts", () => {
    expect(playedFor(times, 60_000, 30)).toBe(2000);
    expect(playedFor(times, 60_000, "frame")).toBe(4 * FRAME_MS);
  });
});

describe("playerKey", () => {
  it("plays, pauses and steps on the screen, a button keeping its Space and a field every key", () => {
    expect([" ", "ArrowLeft", "ArrowRight", "k"].map((key) => playerKey(key, false, "screen"))).toEqual(["toggle", -1, 1, null]);
    expect([" ", "ArrowLeft"].map((key) => playerKey(key, false, "button"))).toEqual([null, -1]);
    expect([" ", "ArrowRight"].map((key) => playerKey(key, false, "field"))).toEqual([null, null]);
  });
  it("leaves a key held with Alt, Ctrl or ⌘ to the browser", () => {
    expect([" ", "ArrowLeft", "ArrowRight"].map((key) => playerKey(key, true, "screen"))).toEqual([null, null, null]);
  });
});
