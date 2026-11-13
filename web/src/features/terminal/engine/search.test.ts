import { describe, expect, it } from "vitest";
import { FLAVORS } from "@/lib/theme";
import { cellSpan, hexAlpha, occurrences, pickNear, searchDecorations } from "./search";

const FRAPPE = FLAVORS.frappe;
const LATTE = FLAVORS.latte;

describe("hexAlpha", () => {
  it("appends the alpha as two hex digits", () => {
    expect(hexAlpha("#e5c890", 1)).toBe("#e5c890ff");
    expect(hexAlpha("#e5c890", 0)).toBe("#e5c89000");
    expect(hexAlpha("#e5c890", 0.6)).toBe("#e5c89099");
  });
  it("keeps the alpha within 0..1", () => {
    expect(hexAlpha("#000000", 2)).toBe("#000000ff");
    expect(hexAlpha("#000000", -1)).toBe("#00000000");
  });
});

describe("searchDecorations", () => {
  it("colours every match in the flavour's yellow, the current one stronger", () => {
    const d = searchDecorations(FRAPPE);
    expect(d.matchBackground).toBe(hexAlpha(FRAPPE.yellow, 0.28));
    expect(d.activeMatchBackground).toBe(hexAlpha(FRAPPE.yellow, 0.6));
    expect(d.matchOverviewRuler).toBe(FRAPPE.yellow);
    expect(searchDecorations(LATTE).matchBackground).not.toBe(d.matchBackground);
  });
});

describe("occurrences", () => {
  it("lists every place on every row, case-folded", () => {
    expect(occurrences(["Error: x", "", "no error, ERROR"], "error")).toEqual([
      { row: 0, col: 0, length: 5 },
      { row: 2, col: 3, length: 5 },
      { row: 2, col: 10, length: 5 },
    ]);
  });
  it("measures a match in the row's own UTF-16 units, so an emoji in the query is marked whole", () => {
    // the server counts "🚀x" as two characters; on the screen it is three units over three cells
    const [hit] = occurrences(["🚀x end"], "🚀x");
    expect(hit).toEqual({ row: 0, col: 0, length: 3 });
    const row = { length: 7, getCell: (x: number) => ({ getWidth: () => [2, 0, 1, 1, 1, 1, 1][x]!, getChars: () => ["🚀", "", "x", " ", "e", "n", "d"][x]! }) } as never;
    expect(cellSpan(row, hit!.col, hit!.length)).toEqual({ col: 0, width: 3 });
  });
});

describe("pickNear", () => {
  const found = [
    { row: 3, col: 0 },
    { row: 25, col: 2 },
    { row: 26, col: 1 },
    { row: 26, col: 14 },
    { row: 27, col: 0 },
  ];
  it("takes the expected row, the column nearest the one asked", () => {
    expect(pickNear(found, 26, 12)).toBe(3);
    expect(pickNear(found, 26, 0)).toBe(2);
  });
  it("takes a neighbouring row when the expected one holds none, and nothing further off", () => {
    expect(pickNear(found, 24, 2)).toBe(1);
    expect(pickNear(found, 10, 0)).toBe(-1);
  });
});

describe("cellSpan", () => {
  // line is a row of cells, each its width and its characters ("x" unless given; a wide character's spacer has none)
  const line = (widths: number[], chars: string[] = []) => ({ length: widths.length, getCell: (x: number) => ({ getWidth: () => widths[x]!, getChars: () => chars[x] ?? (widths[x] ? "x" : "") }) }) as never;
  it("maps characters to cells past wide characters", () => {
    // 日本語 err: three wide characters (each a 2-cell and a 0-cell), a space, then "err" at character 4
    expect(cellSpan(line([2, 0, 2, 0, 2, 0, 1, 1, 1, 1]), 4, 3)).toEqual({ col: 7, width: 3 });
    expect(cellSpan(line([1, 1, 1, 1]), 1, 2)).toEqual({ col: 1, width: 2 });
  });
  it("counts an emoji as the two units it is in the row's text", () => {
    // "🚀 err": the emoji fills cells 0–1 and units 0–1, the space cell 2 and unit 2, "err" cells 3–5 from unit 3
    expect(cellSpan(line([2, 0, 1, 1, 1, 1], ["🚀", "", " ", "e", "r", "r"]), 3, 3)).toEqual({ col: 3, width: 3 });
  });
  it("falls back to the characters without a line", () => {
    expect(cellSpan(undefined, 5, 2)).toEqual({ col: 5, width: 2 });
  });
});
