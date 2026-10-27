import { describe, expect, it } from "vitest";
import { blankTail, clampAt, fromBottom, nearest, offsetFor, onScreen, viewportRow } from "./scrollback";

const scroll = (max: number, rows: number, offset = 0) => ({ offset_from_bottom: offset, max_offset_from_bottom: max, viewport_rows: rows });

describe("blankTail", () => {
  it("is the screen's blank rows under the text when the read was whole", () => {
    expect(blankTail(2, false, scroll(0, 40))).toBe(38); // a fresh prompt at the top of an empty screen
    expect(blankTail(123, false, scroll(83, 40))).toBe(0); // a full screen with 83 rows above it
  });
  it("is none for a truncated read or without the scroll state", () => {
    expect(blankTail(60, true, scroll(83, 40))).toBe(0);
    expect(blankTail(2, false, null)).toBe(0);
  });
});

describe("the rows", () => {
  it("count a match from the bottom of the screen and back to a screen row", () => {
    const fb = fromBottom(100, 123, 0); // row 100 of 123, no blank tail: 22 rows above the bottom
    expect(fb).toBe(22);
    expect(viewportRow(fb, 0, 40)).toBe(17);
    expect(onScreen(viewportRow(fb, 0, 40), 40)).toBe(true);
    expect(onScreen(viewportRow(fromBottom(10, 123, 0), 0, 40), 40)).toBe(false); // 112 rows up: off the top
    expect(viewportRow(22, 30, 40)).toBe(47); // scrolled 30 up, the match went under the bottom
  });
  it("keep the blank tail in the count", () => {
    expect(viewportRow(fromBottom(1, 2, 38), 0, 40)).toBe(1); // the second of two rows on an otherwise empty screen
  });
});

describe("offsetFor", () => {
  it("puts the match a third of the screen above the bottom", () => {
    expect(offsetFor(112, 40)).toBe(99);
    expect(viewportRow(112, offsetFor(112, 40), 40)).toBe(26);
  });
  it("never scrolls past the bottom", () => {
    expect(offsetFor(5, 40)).toBe(0);
  });
});

describe("nearest", () => {
  it("picks the row closest to the one expected, the later on a tie", () => {
    expect(nearest([3, 10, 26, 27], 26)).toBe(2);
    expect(nearest([25, 27], 26)).toBe(1);
    expect(nearest([], 26)).toBe(-1);
  });
});

describe("clampAt", () => {
  it("keeps the index inside the list", () => {
    expect(clampAt(5, 3)).toBe(2);
    expect(clampAt(-1, 3)).toBe(0);
    expect(clampAt(1, 0)).toBe(0);
  });
});
