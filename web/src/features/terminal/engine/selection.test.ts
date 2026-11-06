import { describe, expect, it } from "vitest";
import { lineCells, relocate, sliceRows, type CellLine } from "./selection";

const content = (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, i) => `line ${from + i} of the conversation`);
const memory = (rows: string[], offset = 0) => new Map(rows.map((text, i) => [i + offset, text]));

describe("relocate", () => {
  it("finds the offset most unique rows agree on", () => {
    const seen = memory(content(0, 9));
    expect(relocate([...content(3, 9), "new 10", "new 11", "new 12"], seen, 0)).toBe(3);
    expect(relocate(content(0, 9), seen, 0)).toBe(0);
  });
  it("knows nothing when the screen is somewhere unseen, or too little of it is recognised", () => {
    const seen = memory(content(0, 9));
    expect(relocate(content(20, 29), seen, 0)).toBeNull();
    expect(relocate([...content(8, 9), ...content(30, 37)], seen, 0)).toBeNull();
  });
  it("ignores rows that repeat on screen or in the memory, and short ones", () => {
    const seen = memory(["app header", "a", ...content(0, 7), "app header"]);
    const screen = ["app header", "a", ...content(2, 7), "x", "app header"];
    expect(relocate(screen, seen, 0)).toBe(2);
  });
});

// WIDE are the characters the fake screen draws two cells wide, as xterm's Unicode table does
const WIDE = new Set(["日", "本", "🚀"]);

// line lays a text out in cells the way xterm's buffer does: a wide character, then a spacer cell of width 0
function line(text: string): CellLine {
  const cells: { chars: string; width: number }[] = [];
  for (const ch of text) {
    const width = WIDE.has(ch) ? 2 : 1;
    cells.push({ chars: ch === " " ? "" : ch, width });
    if (width === 2) cells.push({ chars: "", width: 0 });
  }
  return { length: cells.length, getCell: (x) => (cells[x] ? { getChars: () => cells[x]!.chars, getWidth: () => cells[x]!.width } : undefined) };
}

const rowsOf = (texts: string[]) => new Map(texts.map((text, i) => [i, lineCells(line(text))]));

describe("lineCells", () => {
  it("reads a row cell by cell, a wide character's spacer empty, a blank cell a space", () => {
    expect(lineCells(line("a 日b"))).toEqual(["a", " ", "日", "", "b"]);
    expect(lineCells(line("🚀x")).join("")).toBe("🚀x");
  });
});

describe("sliceRows", () => {
  it("cuts the selected text out of the remembered rows, trailing spaces trimmed", () => {
    const rows = rowsOf(["hello world   ", "second row     ", "third"]);
    expect(sliceRows(rows, { row: 0, col: 6 }, { row: 2, col: 3 })).toBe("world\nsecond row\nthi");
    expect(sliceRows(rows, { row: 1, col: 0 }, { row: 1, col: 6 })).toBe("second");
    expect(sliceRows(rows, { row: 2, col: 0 }, { row: 4, col: 0 })).toBe("third\n\n");
  });
  it("counts a selection's columns in cells, a wide character or an emoji filling two", () => {
    // 日 and 本 fill cells 0–3, the space 4, "hello" 5–9; the emoji 0–1, "ok" 3–4
    const rows = rowsOf(["日本 hello", "🚀 ok done"]);
    expect(sliceRows(rows, { row: 0, col: 5 }, { row: 0, col: 10 })).toBe("hello");
    expect(sliceRows(rows, { row: 0, col: 2 }, { row: 1, col: 5 })).toBe("本 hello\n🚀 ok");
  });
});
