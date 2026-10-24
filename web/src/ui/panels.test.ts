import { describe, expect, it } from "vitest";
import { clampWidth, dragWidth, keyWidth, PANELS, STEP } from "./panels";

const spec = PANELS.sidebar; // 268, 200..520

describe("clampWidth", () => {
  it("keeps a width in the range, in whole pixels, and falls back to the usual one", () => {
    expect(clampWidth(spec, 300.4)).toBe(300);
    expect(clampWidth(spec, 10)).toBe(200);
    expect(clampWidth(spec, 9000)).toBe(520);
    expect(clampWidth(spec, NaN)).toBe(268);
    expect(clampWidth(spec, Infinity)).toBe(268);
  });
});

describe("dragWidth", () => {
  it("grows a panel whose right edge moves right and one whose left edge moves left", () => {
    expect(dragWidth(spec, 268, 40, "right")).toBe(308);
    expect(dragWidth(spec, 268, -40, "right")).toBe(228);
    expect(dragWidth(PANELS.rail, 300, 40, "left")).toBe(260);
    expect(dragWidth(PANELS.rail, 300, -40, "left")).toBe(340);
    expect(dragWidth(spec, 268, -500, "right")).toBe(200);
  });
});

describe("keyWidth", () => {
  it("steps with the arrows, more with shift, and jumps with Home, End and Enter", () => {
    expect(keyWidth(spec, 268, "ArrowRight", false, "right")).toBe(268 + STEP);
    expect(keyWidth(spec, 268, "ArrowRight", true, "right")).toBe(268 + STEP * 4);
    expect(keyWidth(spec, 268, "ArrowLeft", false, "right")).toBe(268 - STEP);
    expect(keyWidth(PANELS.rail, 300, "ArrowRight", false, "left")).toBe(300 - STEP);
    expect(keyWidth(spec, 268, "Home", false, "right")).toBe(200);
    expect(keyWidth(spec, 268, "End", false, "right")).toBe(520);
    expect(keyWidth(spec, 400, "Enter", false, "right")).toBe(268);
    expect(keyWidth(spec, 268, "a", false, "right")).toBeNull();
    expect(keyWidth(spec, 268, "Tab", false, "right")).toBeNull();
  });
});

describe("PANELS", () => {
  it("has a pref key, a CSS variable and a sane range each", () => {
    for (const p of Object.values(PANELS)) {
      expect(p.key.startsWith("panel.")).toBe(true);
      expect(p.cssVar.startsWith("--")).toBe(true);
      expect(p.min).toBeLessThan(p.width);
      expect(p.width).toBeLessThan(p.max);
    }
  });
});
