// A screen as a grid of cells for drawing on a canvas: every character in its column, a wide one taking two, its
// colours resolved (the defaults filled in, a dim one mixed toward its background as the screen view's opacity draws
// it), so a row can be painted cell by cell and its pixels told apart as ink or ground. Pure.

import type { Line } from "../ansi";

export type Cell = { ch: string; fg: number; bg: number; bold?: boolean; italic?: boolean; underline?: boolean; strike?: boolean }; // colours as 0xRRGGBB
export type Defaults = { fg: number; bg: number };

// DIM is how much of the ink a dim span keeps, as `.screen .d` draws it
export const DIM = 0.6;

// rgbOf reads a CSS hex colour, #rgb or #rrggbb; anything else is the fallback
export function rgbOf(css: string | undefined, fallback: number): number {
  if (!css) return fallback;
  const hex = css.startsWith("#") ? css.slice(1) : "";
  if (/^[0-9a-f]{6}$/i.test(hex)) return parseInt(hex, 16);
  if (/^[0-9a-f]{3}$/i.test(hex)) return parseInt([...hex].map((c) => c + c).join(""), 16);
  return fallback;
}

// mix is a of the way from b to a: mix(ink, ground, 0.6) is ink at 60 %
export function mix(a: number, b: number, share: number): number {
  const channel = (shift: number) => Math.round(((a >> shift) & 0xff) * share + ((b >> shift) & 0xff) * (1 - share));
  return (channel(16) << 16) | (channel(8) << 8) | channel(0);
}

// charWidth is the columns a character takes: none for a combining mark or a joiner, two for an East Asian wide one or
// an emoji drawn as a picture, one for the rest
export function charWidth(code: number): number {
  if (code === 0x200d || (code >= 0x300 && code <= 0x36f) || (code >= 0xfe00 && code <= 0xfe0f) || (code >= 0x20d0 && code <= 0x20ff)) return 0;
  if (
    (code >= 0x1100 && code <= 0x115f) ||
    (code >= 0x2e80 && code <= 0xa4cf && code !== 0x303f) ||
    (code >= 0xac00 && code <= 0xd7a3) ||
    (code >= 0xf900 && code <= 0xfaff) ||
    (code >= 0xfe30 && code <= 0xfe4f) ||
    (code >= 0xff00 && code <= 0xff60) ||
    (code >= 0xffe0 && code <= 0xffe6) ||
    (code >= 0x1f300 && code <= 0x1f64f) ||
    (code >= 0x1f900 && code <= 0x1f9ff) ||
    (code >= 0x20000 && code <= 0x3fffd)
  )
    return 2;
  return 1;
}

// cellsOf lays a line's spans out in cells; a wide character's second cell holds nothing and wears its colours
export function cellsOf(line: Line, defaults: Defaults): Cell[] {
  const cells: Cell[] = [];
  for (const span of line) {
    const bg = rgbOf(span.bg, defaults.bg);
    const ink = rgbOf(span.fg, defaults.fg);
    const fg = span.dim ? mix(ink, bg, DIM) : ink;
    const style = { ...(span.bold && { bold: true }), ...(span.italic && { italic: true }), ...(span.underline && { underline: true }), ...(span.strike && { strike: true }) };
    for (const ch of span.text) {
      const width = charWidth(ch.codePointAt(0)!);
      if (width === 0 && cells.length > 0) {
        cells[cells.length - 1]!.ch += ch;
        continue;
      }
      cells.push({ ch: ch === "\t" ? " " : ch, fg, bg, ...style });
      if (width === 2) cells.push({ ch: "", fg, bg });
    }
  }
  return cells;
}

// rowKey says whether a row changed between two frames: the same text in the same colours is the same row
export function rowKey(cells: Cell[]): string {
  return cells.map((c) => `${c.ch}\u0001${c.fg}\u0001${c.bg}\u0001${c.bold ? 1 : 0}${c.italic ? 1 : 0}${c.underline ? 1 : 0}${c.strike ? 1 : 0}`).join("\u0002");
}
