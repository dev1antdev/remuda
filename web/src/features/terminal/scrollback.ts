// The find over the scrollback, the page's side: herdr keeps the rows and reports where the viewport stands
// (offset_from_bottom, max_offset_from_bottom, viewport_rows); the server finds the query in the rows it read,
// and the page scrolls the pane so a match is on screen and says which row of the screen to mark.

// SCROLLBACK_ROWS is how many rows one find reads: a build log's tail, not the whole history.
export const SCROLLBACK_ROWS = 5000;

export type Scroll = { offset_from_bottom: number; max_offset_from_bottom: number; viewport_rows: number };
export type ScrollbackMatch = { row: number; col: number; len: number };
export type Found = { matches: ScrollbackMatch[]; rows: number; truncated: boolean; scroll: Scroll | null };

// blankTail counts the screen's blank rows under the last row read: herdr's read ends at the last row with text,
// while the offsets count the whole viewport; a truncated read means the screen is full, so none.
export function blankTail(rows: number, truncated: boolean, scroll: Scroll | null): number {
  if (truncated || !scroll) return 0;
  return Math.max(0, scroll.max_offset_from_bottom + scroll.viewport_rows - rows);
}

// fromBottom is how many rows a match sits above the bottom of the screen when nothing is scrolled.
export function fromBottom(row: number, rows: number, tail: number): number {
  return rows - 1 - row + tail;
}

// viewportRow is the screen row (0 = top) a match shows on at an offset, past the screen's edges when off it.
export function viewportRow(fromBottomRows: number, offset: number, viewportRows: number): number {
  return viewportRows - 1 - (fromBottomRows - offset);
}

// onScreen says whether that row is one of the screen's.
export function onScreen(row: number, viewportRows: number): boolean {
  return row >= 0 && row < viewportRows;
}

// offsetFor is the offset that puts a match a third of the screen above its bottom, never below the bottom.
export function offsetFor(fromBottomRows: number, viewportRows: number): number {
  return Math.max(0, fromBottomRows - Math.floor(viewportRows / 3));
}

// nearest picks the occurrence whose row is closest to the one expected, the later one on a tie; -1 for none.
export function nearest(rows: number[], expected: number): number {
  let best = -1;
  for (let i = 0; i < rows.length; i++) {
    if (best < 0 || Math.abs(rows[i]! - expected) <= Math.abs(rows[best]! - expected)) best = i;
  }
  return best;
}

// clampAt keeps the current match inside a list that may have changed under it.
export function clampAt(at: number, total: number): number {
  return total <= 0 ? 0 : Math.min(Math.max(at, 0), total - 1);
}
