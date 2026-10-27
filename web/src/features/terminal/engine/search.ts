// The find over the scrollback as the screen shows it: xterm's own search addon marks every match on the visible
// rows in the page's amber and selects the current one. The rows themselves are herdr's — the attach stream repaints
// the screen, so xterm never holds a scrollback — and useTerminalFind reads them through the server and scrolls the
// pane; what arrives here is which row of the screen to mark once the repaint has landed.

import { SearchAddon, type ISearchDecorationOptions, type ISearchOptions } from "@xterm/addon-search";
import type { IBufferLine, Terminal as XTerm } from "@xterm/xterm";
import { findRanges } from "@/features/chat/find";
import { swatches, type Flavor, type Swatches } from "@/lib/theme";
import { nearest } from "../scrollback";
import type { SearchApi } from "./types";

// SETTLE_MS is how long the screen gets to repaint after a scroll before the match is looked for, a few frames at a time.
const SETTLE_MS = 1500;
const TICK_MS = 60;

// hexAlpha is a six-digit hex colour with an alpha, as #rrggbbaa, the form a decoration takes
export function hexAlpha(hex: string, alpha: number): string {
  const a = Math.round(Math.min(1, Math.max(0, alpha)) * 255)
    .toString(16)
    .padStart(2, "0");
  return hex + a;
}

// searchDecorations is the find bar's amber on the terminal: every match in a soft fill, the current one stronger
export function searchDecorations(c: Swatches): ISearchDecorationOptions {
  return {
    matchBackground: hexAlpha(c.yellow, 0.28),
    matchBorder: hexAlpha(c.yellow, 0.6),
    matchOverviewRuler: c.yellow,
    activeMatchBackground: hexAlpha(c.yellow, 0.6),
    activeMatchBorder: c.yellow,
    activeMatchColorOverviewRuler: c.peach,
  };
}

export type Occurrence = { row: number; col: number };

// occurrences lists where the query sits on the screen's rows, case-folded, a character index per match
export function occurrences(lines: string[], query: string): Occurrence[] {
  const out: Occurrence[] = [];
  lines.forEach((line, row) => {
    for (const [col] of findRanges(line, query)) out.push({ row, col });
  });
  return out;
}

// pickNear chooses the occurrence on the row expected or a neighbour of it (a screen a row off), nearest the column; -1 for none
export function pickNear(found: Occurrence[], expectedRow: number, col: number): number {
  const rows = found.map((o) => (Math.abs(o.row - expectedRow) <= 1 ? o.row : Number.NaN));
  const row = nearest(rows.filter((r) => !Number.isNaN(r)), expectedRow);
  if (row < 0) return -1;
  const wantRow = rows.filter((r) => !Number.isNaN(r))[row]!;
  let best = -1;
  found.forEach((o, i) => {
    if (o.row !== wantRow) return;
    if (best < 0 || Math.abs(o.col - col) < Math.abs(found[best]!.col - col)) best = i;
  });
  return best;
}

// cellSpan is a run of characters of a row as cells: a wide character fills two
export function cellSpan(line: IBufferLine | undefined, from: number, length: number): { col: number; width: number } {
  if (!line) return { col: from, width: length };
  let chars = 0;
  let start = -1;
  let x = 0;
  for (; x < line.length; x++) {
    const width = line.getCell(x)?.getWidth() ?? 1;
    if (width === 0) continue;
    if (chars === from) start = x;
    if (chars === from + length) break;
    chars++;
  }
  if (start < 0) return { col: from, width: length };
  return { col: start, width: Math.max(1, x - start) };
}

// installSearch loads the addon into the terminal; refresh paints the current find again (a flavour change). A match
// is selected the way the addon does it, and forgetSelection tells the engine's own selection tracker that this one
// is not the person's — else it would carry the mark to wherever that text seems to have moved after a repaint.
export function installSearch(term: XTerm, flavor: () => Flavor, forgetSelection: () => void): SearchApi & { refresh: () => void; dispose: () => void } {
  let addon = new SearchAddon();
  term.loadAddon(addon);
  // fresh replaces the addon before a find: its cache of the rows cannot know that herdr rewrote them in place (the
  // cursor ends where it began, so xterm reports no move), and a stale cache finds the screen as it was
  const fresh = () => {
    addon.dispose();
    addon = new SearchAddon();
    term.loadAddon(addon);
  };
  let query = "";
  let pending: ReturnType<typeof setTimeout> | null = null;
  const options = (): ISearchOptions => ({ incremental: true, decorations: searchDecorations(swatches(flavor())) });
  const screenLines = (): string[] => {
    const buffer = term.buffer.active;
    const out: string[] = [];
    for (let i = 0; i < term.rows; i++) out.push(buffer.getLine(buffer.viewportY + i)?.translateToString(true) ?? "");
    return out;
  };
  // mark selects the occurrence and lets a fresh addon decorate every match on the screen, the selected one current:
  // with no term cached, the find starts at the selection's start rather than after it
  const mark = (row: number, col: number, length: number) => {
    const bufferRow = term.buffer.active.viewportY + row;
    const span = cellSpan(term.buffer.active.getLine(bufferRow), col, length);
    fresh();
    term.select(span.col, bufferRow, span.width);
    addon.findNext(query, options());
    forgetSelection();
  };
  const stop = () => {
    if (pending) clearTimeout(pending);
    pending = null;
  };
  const clear = () => {
    stop();
    query = "";
    addon.clearDecorations();
    term.clearSelection();
  };
  return {
    show(next, expectedRow, col, length) {
      stop();
      query = next;
      const started = Date.now();
      const attempt = () => {
        pending = null;
        const found = occurrences(screenLines(), query);
        const pick = pickNear(found, expectedRow, col);
        if (pick >= 0) {
          mark(found[pick]!.row, found[pick]!.col, length);
          return;
        }
        if (Date.now() - started < SETTLE_MS) {
          pending = setTimeout(attempt, TICK_MS);
          return;
        }
        // the screen never showed that row: what is on it is marked all the same, the first match current
        fresh();
        term.clearSelection();
        addon.findNext(query, options());
        forgetSelection();
      };
      attempt();
    },
    clear,
    refresh() {
      if (!query) return;
      fresh();
      addon.findNext(query, options());
      forgetSelection();
    },
    dispose() {
      stop();
      addon.dispose();
    },
  };
}
