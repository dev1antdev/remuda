import { describe, expect, it } from "vitest";
import { dropAfter, markerSlot } from "./drop";

// a column of three cards; the pointer's index counts the other cards only, so the dragged card's own row is skipped
const column = [1, 2, 3];

describe("dropAfter", () => {
  it("moves a card one row down in its own column", () => {
    expect(dropAfter(column, 2, 2)).toEqual({ after: 3 });
    expect(dropAfter([1, 2], 1, 1)).toEqual({ after: 2 });
  });
  it("moves a card to the top and to the end", () => {
    expect(dropAfter(column, 2, 0)).toEqual({ after: 0 });
    expect(dropAfter(column, 1, 2)).toEqual({ after: 3 });
  });
  it("knows the slots either side of the card are where it already sits", () => {
    expect(dropAfter(column, 2, 1)).toBeNull();
    expect(dropAfter(column, 1, 0)).toBeNull();
    expect(dropAfter(column, 3, 2)).toBeNull();
  });
  it("lands in another column behind the card above the pointer, or at its top or end", () => {
    expect(dropAfter([4, 5], 2, 0)).toEqual({ after: 0 });
    expect(dropAfter([4, 5], 2, 1)).toEqual({ after: 4 });
    expect(dropAfter([4, 5], 2, 2)).toEqual({ after: 5 });
    expect(dropAfter([4, 5], 2, 9)).toEqual({ after: 5 });
    expect(dropAfter([], 2, 0)).toEqual({ after: 0 });
  });
});

describe("markerSlot", () => {
  it("draws the marker above the row the drop would take, the dragged card's own row counted past it", () => {
    expect(markerSlot(column, 2, 0)).toBe(0);
    expect(markerSlot(column, 2, 1)).toBe(1);
    expect(markerSlot(column, 2, 2)).toBe(3); // below the last card: the end marker, not a line above it
  });
  it("is the index itself in another column", () => {
    expect(markerSlot([4, 5], 2, 0)).toBe(0);
    expect(markerSlot([4, 5], 2, 2)).toBe(2);
  });
});
