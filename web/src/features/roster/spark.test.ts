import { describe, expect, it } from "vitest";
import { areaPath, DISK_THRESHOLDS, loadThresholds, rangeOf, sparkPath, sparkTitle, spanWords, thresholdY } from "./spark";

describe("sparkPath", () => {
  it("lays the values left to right, the floor at the bottom and the ceiling at the top", () => {
    expect(sparkPath([0, 5, 10], 100, 20, 0, 10)).toBe("M0 20 L50 10 L100 0");
  });
  it("draws one value as a flat line and none as nothing", () => {
    expect(sparkPath([2.5], 100, 20, 0, 10)).toBe("M0 15 L100 15");
    expect(sparkPath([], 100, 20, 0, 10)).toBe("");
  });
  it("keeps a value past the range on the edge", () => {
    expect(sparkPath([20, -3], 100, 20, 0, 10)).toBe("M0 0 L100 20");
  });
  it("closes the line down to the floor for the fill", () => {
    expect(areaPath("M0 20 L100 0", 100, 20)).toBe("M0 20 L100 0 L100 20 L0 20 Z");
    expect(areaPath("", 100, 20)).toBe("");
  });
});

describe("thresholdY and rangeOf", () => {
  it("places a threshold inside the range and drops one outside it", () => {
    expect(thresholdY(5, 0, 10, 20)).toBe(10);
    expect(thresholdY(15, 0, 10, 20)).toBeNull();
  });
  it("draws up to the ceiling, or a little past the highest value when it goes higher", () => {
    expect(rangeOf([1, 2.5], 0, 10)).toEqual({ min: 0, max: 10 });
    expect(rangeOf([12, 30], 0, 10)).toEqual({ min: 0, max: 31.5 });
    expect(rangeOf([], 0, 0)).toEqual({ min: 0, max: 1 });
  });
});

describe("thresholds", () => {
  it("marks one and two per core for the load, nothing without a core count", () => {
    expect(loadThresholds(10)).toEqual([
      { value: 10, tone: "amber" },
      { value: 20, tone: "red" },
    ]);
    expect(loadThresholds(undefined)).toEqual([]);
    expect(loadThresholds(0)).toEqual([]);
  });
  it("keeps the disk's own", () => {
    expect(DISK_THRESHOLDS.map((t) => t.value)).toEqual([10, 5]);
  });
});

describe("words", () => {
  it("says the span in hours or minutes", () => {
    expect(spanWords("2026-10-27T08:00:00Z", "2026-10-28T06:00:00Z")).toBe("last 22 h");
    expect(spanWords("2026-10-27T08:00:00Z", "2026-10-27T08:30:00Z")).toBe("last 30 min");
    expect(spanWords("2026-10-27T08:00:00Z", "2026-10-27T07:00:00Z")).toBe("");
    expect(spanWords("nope", "2026-10-27T07:00:00Z")).toBe("");
  });
  it("titles the chart with the least, the most and the latest", () => {
    expect(sparkTitle("Load", [0.8, 6.5, 2.1], (v) => v.toFixed(2), "last 22 h")).toBe("Load: min 0.80, max 6.50, now 2.10 · last 22 h");
    expect(sparkTitle("Disk free", [59], (v) => `${v}%`, "")).toBe("Disk free: min 59%, max 59%, now 59%");
    expect(sparkTitle("Load", [], String, "")).toBe("Load");
  });
});
