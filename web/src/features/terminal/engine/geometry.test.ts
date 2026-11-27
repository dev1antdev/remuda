import { describe, expect, it } from "vitest";
import { colsIn, keepFont, MIN_FONT, rowsIn, thumbFont } from "./geometry";

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

describe("thumbFont", () => {
  it("fills the width with the pane's columns, never larger than the base size, in half-pixel steps", () => {
    expect(thumbFont(1000, 80, 7.8, 13)).toBe(13);
    expect(thumbFont(440, 110, 7.8, 13)).toBe(6.5);
    expect(thumbFont(300, 200, 7.8, 13)).toBe(2.5);
    expect(thumbFont(10, 200, 7.8, 13)).toBe(2);
    expect(thumbFont(100, 0, 7.8, 13)).toBe(13);
  });
});

describe("the grid's bounds", () => {
  it("keeps a pane between 20 and 500 columns, 5 and 300 rows", () => {
    expect(colsIn(2)).toBe(20);
    expect(colsIn(80)).toBe(80);
    expect(colsIn(900)).toBe(500);
    expect(rowsIn(1)).toBe(5);
    expect(rowsIn(40)).toBe(40);
    expect(rowsIn(900)).toBe(300);
  });
});
