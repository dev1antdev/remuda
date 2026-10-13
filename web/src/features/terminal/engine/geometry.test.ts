import { describe, expect, it } from "vitest";
import { keepFont, MIN_FONT } from "./geometry";

describe("keepFont", () => {
  it("keeps the preferred size when the grid fits", () => {
    expect(keepFont(2000, 13, 120, 7.8, 13)).toBe(13);
  });
  it("shrinks the font to fit the pane's columns in the width", () => {
    expect(keepFont(800, 13, 120, 7.8, 13)).toBe(11);
  });
  it("never goes under the minimum", () => {
    expect(keepFont(300, 13, 200, 7.8, 13)).toBe(MIN_FONT);
  });
});
