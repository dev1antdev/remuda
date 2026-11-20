import { describe, expect, it } from "vitest";
import { Terminal, type IBufferRange } from "@xterm/xterm";
import { FLAVORS } from "@/lib/theme";
import { cellSpan, hexAlpha, installSearch, occurrences, pickNear, searchDecorations } from "./search";
import type { FoundCount } from "./types";

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

// headless is a real xterm with the selection a mounted one keeps, which the search addon selects its matches with
function headless(cols = 40, rows = 5): Terminal {
  const term = new Terminal({ allowProposedApi: true, cols, rows, scrollback: 100 });
  let range: IBufferRange | undefined;
  const select = (col: number, row: number, length: number) => {
    const end = row * cols + col + length;
    range = { start: { x: col, y: row }, end: { x: end % cols, y: Math.floor(end / cols) } };
  };
  const own: Record<string, unknown> = { select, getSelectionPosition: () => range, clearSelection: () => (range = undefined) };
  return new Proxy(term, { get: (target, key, receiver) => (typeof key === "string" && key in own ? own[key] : Reflect.get(target, key, receiver)) });
}

const written = (term: Terminal, data: string) => new Promise<void>((done) => term.write(data, done));
const rest = (ms: number) => new Promise((done) => setTimeout(done, ms));
// rewrite puts text at the start of a row and the cursor back where it was, as a program redrawing a line does
const rewrite = (row: number, text: string) => `\x1b7\x1b[${row};1H${text}\x1b8`;

describe("installSearch on remuda-core", () => {
  it("counts a row rewritten in place once output rests", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {});
    await written(term, "OLD row\r\nsecond\r\n");
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    expect(found.total).toBe(0);
    await written(term, rewrite(1, "NEW"));
    await rest(350);
    expect(found.total).toBe(1);
    search.dispose();
  });
  it("steps over a row rewritten in place since the last step", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {});
    await written(term, "NEW a\r\nOLD b\r\n");
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    expect(found).toEqual({ total: 1, at: 0 });
    await written(term, rewrite(2, "NEW"));
    search.find!("NEW", 1, (f) => (found = f));
    expect(found).toEqual({ total: 2, at: 1 });
    search.find!("NEW", -1, (f) => (found = f));
    expect(found).toEqual({ total: 2, at: 0 });
    search.dispose();
  });
  it("steps back once when the match selected was rewritten away", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {});
    await written(term, "NEW 1\r\nNEW 2\r\nNEW 3\r\n");
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    search.find!("NEW", 1, (f) => (found = f));
    search.find!("NEW", 1, (f) => (found = f));
    expect(found).toEqual({ total: 3, at: 2 });
    await written(term, rewrite(3, "OLD"));
    search.find!("NEW", -1, (f) => (found = f));
    expect(found).toEqual({ total: 2, at: 1 });
    search.dispose();
  });
  it("steps back past a line wrapped onto the next row when the match selected was rewritten away", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {});
    await written(term, `NEW a\r\nxx NEW ${"y".repeat(36)}NEW tail\r\n`);
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    search.find!("NEW", 1, (f) => (found = f));
    expect(found).toEqual({ total: 3, at: 1 });
    await written(term, rewrite(2, "xx OLD"));
    search.find!("NEW", -1, (f) => (found = f));
    expect(found).toEqual({ total: 2, at: 0 });
    search.dispose();
  });
  it("steps back along the buffer's last row when the match selected there was rewritten away", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {});
    await written(term, "a\r\nb\r\nc\r\nd\r\nNEW 1 NEW 2 NEW 3");
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    search.find!("NEW", 1, (f) => (found = f));
    search.find!("NEW", 1, (f) => (found = f));
    expect(found).toEqual({ total: 3, at: 2 });
    await written(term, rewrite(5, "NEW 1 NEW 2 OLD 3"));
    search.find!("NEW", -1, (f) => (found = f));
    expect(found).toEqual({ total: 2, at: 1 });
    search.dispose();
  });
  it("steps back round to the last match when the first was rewritten away", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {});
    await written(term, "NEW 1\r\nNEW 2\r\nNEW 3\r\n");
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    await written(term, rewrite(1, "OLD"));
    search.find!("NEW", -1, (f) => (found = f));
    expect(found).toEqual({ total: 2, at: 1 });
    search.dispose();
  });
});
