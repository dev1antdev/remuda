import { describe, expect, it } from "vitest";
import { NONE, isSelected, pickHow, prune, range, selectionWords, toggle } from "./selection";

const click = (mods: Partial<{ metaKey: boolean; ctrlKey: boolean; shiftKey: boolean }> = {}) => ({ metaKey: false, ctrlKey: false, shiftKey: false, ...mods });

describe("pickHow", () => {
  it("reads the modifiers, and a phone's selection mode", () => {
    expect(pickHow(click(), false)).toBe("open");
    expect(pickHow(click({ metaKey: true }), false)).toBe("toggle");
    expect(pickHow(click({ ctrlKey: true }), false)).toBe("toggle");
    expect(pickHow(click({ shiftKey: true }), false)).toBe("range");
    expect(pickHow(click({ shiftKey: true, metaKey: true }), false)).toBe("range");
    expect(pickHow(click(), true)).toBe("toggle");
  });
});

describe("toggle and range", () => {
  it("toggles one and moves the anchor", () => {
    const a = toggle(NONE, 3);
    expect(a).toEqual({ ids: [3], anchor: 3 });
    expect(isSelected(a, 3)).toBe(true);
    const b = toggle(a, 5);
    expect(b.ids).toEqual([3, 5]);
    expect(b.anchor).toBe(5);
    const c = toggle(b, 3);
    expect(c).toEqual({ ids: [5], anchor: 3 });
  });

  it("ranges from the anchor over the rows as listed, either way, keeping what was selected", () => {
    const listed = [10, 11, 12, 13, 14, 15];
    const a = toggle(NONE, 12);
    const down = range(a, listed, 14);
    expect(down.ids).toEqual([12, 13, 14]);
    expect(down.anchor).toBe(12);
    const up = range(down, listed, 10);
    expect(up.ids).toEqual([12, 13, 14, 10, 11]);
    expect(up.anchor).toBe(12);
  });

  it("falls back to a toggle without an anchor on the list", () => {
    const listed = [1, 2, 3];
    expect(range(NONE, listed, 2)).toEqual({ ids: [2], anchor: 2 });
    const elsewhere = { ids: [9], anchor: 9 };
    expect(range(elsewhere, listed, 3)).toEqual({ ids: [9, 3], anchor: 3 });
  });
});

describe("prune", () => {
  it("drops the tasks that left the list and the anchor with them, and keeps the object when nothing left", () => {
    const sel = { ids: [1, 2, 3], anchor: 2 };
    expect(prune(sel, [1, 2, 3, 4])).toBe(sel);
    expect(prune(sel, [1, 3])).toEqual({ ids: [1, 3], anchor: null });
    expect(prune(sel, [])).toEqual({ ids: [], anchor: null });
  });
});

describe("words", () => {
  it("counts", () => {
    expect(selectionWords(3)).toBe("3 selected");
  });
});
