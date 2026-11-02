import { describe, expect, it } from "vitest";
import type { IBufferCell, IBufferLine, ILink, ILinkProvider, Terminal } from "@xterm/xterm";
import { provideLinks } from "./links";

// WIDE are the characters the fake screen draws two cells wide, as xterm's Unicode table does
const WIDE = new Set(["日", "本", "🚀"]);

// row lays a text out in cells the way xterm's buffer does: a wide character, then an empty spacer cell of width 0
function row(text: string): IBufferLine {
  const cells: { chars: string; width: number }[] = [];
  for (const ch of text) {
    const width = WIDE.has(ch) ? 2 : 1;
    cells.push({ chars: ch, width });
    if (width === 2) cells.push({ chars: "", width: 0 });
  }
  const cell = (x: number): IBufferCell | undefined => (cells[x] ? ({ getChars: () => cells[x]!.chars, getWidth: () => cells[x]!.width } as IBufferCell) : undefined);
  return {
    length: cells.length,
    getCell: cell,
    // xterm's own: each cell's characters, a spacer skipped, trailing blanks dropped on request
    translateToString: (trimRight?: boolean) => {
      const text = cells
        .filter((c) => c.width > 0)
        .map((c) => c.chars || " ")
        .join("");
      return trimRight ? text.trimEnd() : text;
    },
  } as IBufferLine;
}

// linksOn is what the provider hands xterm for a row of an agent's pane, as [start x, end x, text] (1-based cells, inclusive)
function linksOn(text: string): [number, number, string][] {
  let provider: ILinkProvider | null = null;
  const term = {
    buffer: { active: { getLine: () => row(text) } },
    registerLinkProvider: (p: ILinkProvider) => {
      provider = p;
      return { dispose: () => {} };
    },
  } as unknown as Terminal;
  provideLinks(term, () => ({ agent: true, onOpenFile: () => {} }), { linkUsed: false });
  let found: ILink[] = [];
  (provider as ILinkProvider | null)!.provideLinks(1, (links) => (found = links ?? []));
  return found.map((link) => [link.range.start.x, link.range.end.x, link.text]);
}

describe("provideLinks", () => {
  it("places a link on its cells as they are on a plain row", () => {
    expect(linksOn("see https://x.io/a")).toEqual([[5, 18, "https://x.io/a"]]);
  });

  it("counts a wide character before a link as the two cells it fills", () => {
    // 日 and 本 take cells 1–4, the space 5, the URL starts on cell 6
    expect(linksOn("日本 https://x.io/a")).toEqual([[6, 19, "https://x.io/a"]]);
  });

  it("counts a character of two UTF-16 units that fills one cell as one cell", () => {
    expect(linksOn("𝑥 https://x.io")).toEqual([[3, 14, "https://x.io"]]);
  });

  it("ends a link on the last cell of a wide character inside it", () => {
    // "see " cells 1–4, "docs/" 5–9, 日 10–11, 本 12–13, ".md" 14–16
    expect(linksOn("see docs/日本.md")).toEqual([[5, 16, "docs/日本.md"]]);
  });

  it("keeps an emoji, two units on two cells, where it was", () => {
    expect(linksOn("🚀 web/src/a.ts")).toEqual([[4, 15, "web/src/a.ts"]]);
  });
});
