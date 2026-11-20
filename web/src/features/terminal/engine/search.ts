// The find over the scrollback as the screen shows it: xterm's own search addon marks every match on the visible
// rows in the page's amber and selects the current one. On herdr the rows are herdr's — the attach stream repaints
// the screen, so xterm never holds a scrollback — and useTerminalFind reads them through the server and scrolls the
// pane; what arrives here is which row of the screen to mark once the repaint has landed. On remuda-core xterm holds
// the terminal's own rows (the attach paints the history first, the output after it is the program's), so find lets
// the addon look through xterm's buffer itself, count, step and scroll xterm, never the pane.

import { SearchAddon, type ISearchDecorationOptions, type ISearchOptions } from "@xterm/addon-search";
import type { IBufferLine, IBufferRange, Terminal as XTerm } from "@xterm/xterm";
import { findRanges } from "@/features/chat/find";
import { swatches, type Flavor, type Swatches } from "@/lib/theme";
import { nearest } from "../scrollback";
import type { FoundCount, SearchApi } from "./types";

// SETTLE_MS is how long the screen gets to repaint after a scroll before the match is looked for, a few frames at a time.
const SETTLE_MS = 1500;
const TICK_MS = 60;
// RECOUNT_MS is how long output rests before the core find counts its query again: under the addon's own 200 ms
// recount, whose addon it replaces first, so a count from rows read before the write never stands
const RECOUNT_MS = 100;

// samePlace says two selections start at one cell
const samePlace = (a: IBufferRange | undefined, b: IBufferRange | undefined): boolean => !!a && !!b && a.start.x === b.start.x && a.start.y === b.start.y;

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

export type Occurrence = { row: number; col: number; length: number };

// occurrences lists where the query sits on the screen's rows, case-folded, each match's start and length in UTF-16 units
// of its row's text — what cellSpan reads, where the server counts characters
export function occurrences(lines: string[], query: string): Occurrence[] {
  const out: Occurrence[] = [];
  lines.forEach((line, row) => {
    for (const [col, end] of findRanges(line, query)) out.push({ row, col, length: end - col });
  });
  return out;
}

// pickNear chooses the occurrence on the row expected or a neighbour of it (a screen a row off), nearest the column; -1 for none
export function pickNear(found: Pick<Occurrence, "row" | "col">[], expectedRow: number, col: number): number {
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

// cellSpan is a run of a row's text as cells — from and length in UTF-16 units of the row's text, as translateToString
// gives it: a wide character fills two cells, a character of two units (an emoji, a surrogate pair) counts two units
export function cellSpan(line: IBufferLine | undefined, from: number, length: number): { col: number; width: number } {
  if (!line) return { col: from, width: length };
  let chars = 0;
  let start = -1;
  let x = 0;
  for (; x < line.length; x++) {
    const cell = line.getCell(x);
    if (cell?.getWidth() === 0) continue;
    if (chars === from) start = x;
    if (chars === from + length) break;
    chars += (cell?.getChars() || " ").length;
  }
  if (start < 0) return { col: from, width: length };
  return { col: start, width: Math.max(1, x - start) };
}

// installSearch loads the addon into the terminal; refresh paints the current find again (a flavour change). A match
// is selected the way the addon does it, and forgetSelection tells the engine's own selection tracker that this one
// is not the person's — else it would carry the mark to wherever that text seems to have moved after a repaint.
export function installSearch(term: XTerm, flavor: () => Flavor, forgetSelection: () => void): SearchApi & { refresh: () => void; dispose: () => void } {
  let addon = new SearchAddon();
  // heard is the core find's ear: every count the addon gives, its own find's and each it redoes as output arrives
  let heard: ((found: FoundCount) => void) | null = null;
  const listen = () => addon.onDidChangeResults((r) => heard?.({ total: Math.max(0, r.resultCount), at: Math.max(0, r.resultIndex) }));
  term.loadAddon(addon);
  listen();
  // fresh replaces the addon before a find: its cache of the rows cannot know that a row was rewritten in place (herdr's
  // repaint, or a program on remuda-core redrawing a line and putting the cursor back: xterm reports no move), and a
  // stale cache finds the screen as it was
  const fresh = () => {
    addon.dispose();
    addon = new SearchAddon();
    term.loadAddon(addon);
    listen();
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
  // mark selects the occurrence at a row of the buffer and lets a fresh addon decorate every match, the selected one
  // current: with no term cached, the find starts at the selection's start rather than after it
  const mark = (bufferRow: number, col: number, length: number) => {
    const span = cellSpan(term.buffer.active.getLine(bufferRow), col, length);
    fresh();
    term.select(span.col, bufferRow, span.width);
    addon.findNext(query, options());
    forgetSelection();
  };
  // seek selects the next match, or the previous, and tells the engine the selection is not the person's
  const seek = (back = false): boolean => {
    const hit = back ? addon.findPrevious(query, options()) : addon.findNext(query, options());
    forgetSelection();
    return hit;
  };
  // restart starts the find over with a fresh addon from the top of the buffer
  const restart = () => {
    fresh();
    term.clearSelection();
  };
  // again finds the query with a fresh addon from the match selected: that match found once more (or the next, when it
  // is gone), then the step taken from it
  const again = (step: 0 | 1 | -1): boolean => {
    const was = term.getSelectionPosition();
    fresh();
    let hit = seek(step < 0);
    const moved = !samePlace(term.getSelectionPosition(), was);
    if (hit && was && (step < 0 || (step > 0 && !moved))) hit = seek(step < 0);
    return hit;
  };
  let recount: ReturnType<typeof setTimeout> | null = null;
  // the core find counts its query again once output rests, where the screen stands: a write may rewrite rows it read
  const written = term.onWriteParsed(() => {
    if (!heard || !query) return;
    if (recount) clearTimeout(recount);
    recount = setTimeout(() => {
      recount = null;
      if (!heard || !query) return;
      const top = term.buffer.active.viewportY;
      if (!again(0)) heard({ total: 0, at: 0 });
      term.scrollToLine(top);
    }, RECOUNT_MS);
  });
  const stop = () => {
    if (pending) clearTimeout(pending);
    if (recount) clearTimeout(recount);
    pending = recount = null;
  };
  const clear = () => {
    stop();
    heard = null;
    query = "";
    addon.clearDecorations();
    term.clearSelection();
  };
  return {
    show(next, expectedRow, col) {
      stop();
      query = next;
      const started = Date.now();
      const attempt = () => {
        pending = null;
        const found = occurrences(screenLines(), query);
        const pick = pickNear(found, expectedRow, col);
        if (pick >= 0) {
          mark(term.buffer.active.viewportY + found[pick]!.row, found[pick]!.col, found[pick]!.length);
          return;
        }
        if (Date.now() - started < SETTLE_MS) {
          pending = setTimeout(attempt, TICK_MS);
          return;
        }
        // the screen never showed that row: what is on it is marked all the same, the first match current
        restart();
        seek();
      };
      attempt();
    },
    find(next, step, hear) {
      stop();
      heard = hear;
      if (step !== 0 && next === query) {
        if (!again(step)) hear({ total: 0, at: 0 });
        return;
      }
      restart();
      query = next;
      if (!seek(step < 0)) hear({ total: 0, at: 0 });
    },
    clear,
    refresh() {
      if (!query) return;
      fresh();
      seek();
    },
    dispose() {
      stop();
      written.dispose();
      addon.dispose();
    },
  };
}
