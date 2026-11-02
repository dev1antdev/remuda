import { describe, expect, it } from "vitest";
import { gridScrolls, panOf, type GridBox } from "./pointer";

const fits: GridBox = { scrollWidth: 390, clientWidth: 390, scrollHeight: 600, clientHeight: 600, keep: true };
const wide: GridBox = { ...fits, scrollWidth: 960 };
const tall: GridBox = { ...fits, scrollHeight: 900 };

describe("gridScrolls", () => {
  it("gives a grid wider than the screen its sideways gestures, in keep and in a wide fit", () => {
    expect(gridScrolls(true, wide)).toBe(true);
    expect(gridScrolls(true, { ...wide, keep: false })).toBe(true);
    expect(gridScrolls(true, fits)).toBe(false);
  });
  it("gives a keep-mode grid taller than the screen its up and down; a fit grid never scrolls so", () => {
    expect(gridScrolls(false, tall)).toBe(true);
    expect(gridScrolls(false, { ...tall, keep: false })).toBe(false);
    expect(gridScrolls(false, wide)).toBe(false);
  });
});

describe("panOf", () => {
  it("pans a wide grid across under a sideways finger, as the wheel scrolls it", () => {
    expect(panOf(20, 3, wide)).toBe("x");
    expect(panOf(-20, 3, wide)).toBe("x");
  });
  it("pans a tall keep-mode grid down under an upright finger", () => {
    expect(panOf(2, 18, tall)).toBe("y");
  });
  it("leaves the gesture to the app when the grid fits that way", () => {
    expect(panOf(2, 18, wide)).toBe("app");
    expect(panOf(20, 3, tall)).toBe("app");
    expect(panOf(0, 0, fits)).toBe("app");
  });
});
