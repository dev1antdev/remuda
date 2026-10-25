import { describe, expect, it } from "vitest";
import { centerDelta, inView, unmeasured } from "./reveal";

const box = { top: 100, bottom: 700, height: 600 };

describe("reveal geometry", () => {
  it("centres a hit below the box by scrolling down", () => {
    expect(centerDelta({ top: 900, bottom: 920, height: 20 }, box)).toBe(510);
  });
  it("centres a hit above the box by scrolling up", () => {
    expect(centerDelta({ top: -200, bottom: -180, height: 20 }, box)).toBe(-590);
  });
  it("knows a hit inside the box", () => {
    expect(inView({ top: 300, bottom: 320, height: 20 }, box)).toBe(true);
    expect(inView({ top: 690, bottom: 710, height: 20 }, box)).toBe(false);
    expect(inView({ top: 90, bottom: 110, height: 20 }, box)).toBe(false);
  });
  it("tells a rect with no layout from a real one at the origin", () => {
    expect(unmeasured({ top: 0, bottom: 0, height: 0 })).toBe(true);
    expect(unmeasured({ top: 0, bottom: 18, height: 18 })).toBe(false);
    expect(unmeasured({ top: 40, bottom: 40, height: 0 })).toBe(false);
  });
});
