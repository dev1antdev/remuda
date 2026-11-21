import { describe, expect, it } from "vitest";
import { Terminal, type IBufferRange } from "@xterm/xterm";
import { FLAVORS } from "@/lib/theme";
import { hexAlpha, installSearch, searchDecorations } from "./search";
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
    search.onPainted();
    await rest(350);
    expect(found.total).toBe(1);
    search.dispose();
  });
  it("leaves a selection the person holds where it is", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {}, () => true);
    await written(term, "OLD row\r\nsecond\r\n");
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    term.select(0, 1, 6);
    await written(term, rewrite(1, "NEW"));
    search.onPainted();
    await rest(350);
    expect(term.getSelectionPosition()).toEqual({ start: { x: 0, y: 1 }, end: { x: 6, y: 1 } });
    expect(found.total).toBe(0);
    search.dispose();
  });
  it("leaves a selection the person holds where it is through a resize", async () => {
    const term = headless();
    const search = installSearch(term, () => "frappe", () => {}, () => true);
    await written(term, "NEW row\r\nsecond\r\n");
    search.find!("NEW", 0, () => {});
    term.select(0, 1, 6);
    term.resize(50, 5);
    await rest(350);
    expect(term.getSelectionPosition()).toEqual({ start: { x: 0, y: 1 }, end: { x: 6, y: 1 } });
    search.dispose();
  });
  it("counts again once the person lets a held selection go", async () => {
    const term = headless();
    let held = true;
    const search = installSearch(term, () => "frappe", () => {}, () => held);
    await written(term, "OLD row\r\nsecond\r\n");
    let found: FoundCount = { total: -1, at: -1 };
    search.find!("NEW", 0, (f) => (found = f));
    term.select(0, 1, 6);
    await written(term, rewrite(1, "NEW"));
    search.onPainted();
    await rest(350);
    expect(found.total).toBe(0);
    held = false;
    term.clearSelection();
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
