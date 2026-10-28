import { describe, expect, it } from "vitest";
import type { Span } from "./api";
import { hourMarks, layout, spanWords, WIDTH, window } from "./strip";

const HOUR = 3600_000;
// a local morning, so the hour marks fall where the clock says whatever the zone
const eight = new Date(2026, 9, 24, 8, 0).getTime();
const iso = (ms: number) => new Date(ms).toISOString();

const spans: Span[] = [
  { from: iso(eight - HOUR), to: iso(eight + HOUR / 2), state: "working", turn: "u1" },
  { from: iso(eight + HOUR / 2), to: iso(eight + HOUR), state: "waiting", turn: "u1" },
  { from: iso(eight + HOUR), to: iso(eight + 3 * HOUR), state: "idle" },
  { from: iso(eight + 5 * HOUR), to: iso(eight + 6 * HOUR), state: "working", turn: "u2" },
];

describe("window", () => {
  it("runs from the period's start to now", () => {
    expect(window(iso(eight), eight + 4 * HOUR)).toEqual({ from: eight, to: eight + 4 * HOUR });
  });
  it("keeps the last hours on a small screen and a day without a start", () => {
    expect(window(iso(eight), eight + 12 * HOUR, 8)).toEqual({ from: eight + 4 * HOUR, to: eight + 12 * HOUR });
    expect(window(undefined, eight)).toEqual({ from: eight - 24 * HOUR, to: eight });
    expect(window("garbage", eight)).toEqual({ from: eight - HOUR, to: eight });
  });
});

describe("layout", () => {
  it("places the spans on the bar, trimming at the edges and dropping what lies outside", () => {
    const segs = layout(spans, eight, eight + 4 * HOUR);
    expect(segs.map((s) => [s.state, Math.round(s.x), Math.round(s.w), s.turn])).toEqual([
      ["working", 0, 125, "u1"],
      ["waiting", 125, 125, "u1"],
      ["idle", 250, 500, undefined],
    ]);
    expect(segs[0]!.from).toBe(eight);
    expect(layout(spans, eight + 10 * HOUR, eight + 12 * HOUR)).toEqual([]);
  });
});

describe("hourMarks", () => {
  it("marks every whole hour inside the window and labels every third", () => {
    const marks = hourMarks(eight + HOUR / 2, eight + 4 * HOUR);
    expect(marks.map((m) => [Math.round(m.x), m.labelled])).toEqual([
      [Math.round(WIDTH / 7), true], // 9, half an hour into three and a half
      [Math.round((3 * WIDTH) / 7), false], // 10
      [Math.round((5 * WIDTH) / 7), false], // 11
      [WIDTH, true], // 12, the window's end
    ]);
    expect(hourMarks(eight, eight + HOUR, 6).map((m) => m.labelled)).toEqual([false, false]);
  });
});

describe("spanWords", () => {
  it("names the state, the times and the length", () => {
    expect(spanWords({ state: "working", from: eight, to: eight + 46 * 60_000 })).toMatch(/^Working .+–.+, 46 min$/);
    expect(spanWords({ state: "waiting", from: eight, to: eight + 30_000 })).toMatch(/^Waiting for you .+, 30 s$/);
    expect(spanWords({ state: "idle", from: eight, to: eight + 2 * HOUR })).toMatch(/^Idle .+, 2 h$/);
  });
});
