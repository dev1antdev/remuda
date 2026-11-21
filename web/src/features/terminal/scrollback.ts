// The find over the scrollback, the page's side: herdr keeps the rows and reports where the viewport stands
// (offset_from_bottom, max_offset_from_bottom, viewport_rows); the server finds the query in the rows it read,
// and the page scrolls the pane so a match is on screen and says which row of the screen to mark.

// SCROLLBACK_ROWS is how many rows one find reads: a build log's tail, not the whole history.
export const SCROLLBACK_ROWS = 5000;

// XTERM_SCROLLBACK is how many rows above the screen the page's xterm keeps; on remuda-core the attach paints up to as many
// (attachHistory in internal/core/viewers.go), so the find reads the terminal's own rows.
export const XTERM_SCROLLBACK = 2000;

export type ScrollbackMatch = { row: number; col: number; len: number };
export type Found = { matches: ScrollbackMatch[]; rows: number; truncated: boolean };

// blankTail counts the screen's blank rows under the last row read: herdr's read ends at the last row with text,







// OlderRead is the one read, for a pane and query, of the rows older than a terminal on remuda-core holds for its
// screen: every step of the query awaits it rather than reading again or skipping it, and it is told once.
export type OlderRead = { key: string; matches: Promise<number>; told: boolean };

// olderRead keeps the read held while the key is the same, and starts one for a new key.
export function olderRead(held: OlderRead | null, key: string, read: () => Promise<number>): OlderRead {
  return held?.key === key ? held : { key, matches: read(), told: false };
}

// tellsOlder says whether the older rows' matches are news: the run asking is still the current one, this read has not
// been told, the older rows hold the query, and the terminal's own rows still hold none.
export function tellsOlder(current: boolean, read: OlderRead, older: number, counted: number): boolean {
  return current && !read.told && older > 0 && counted === 0;
}

// nearest takes the expected row, the column nearest the one asked (the later on a tie).
export function nearest(rows: number[], expected: number): number {
  let best = -1;
  for (let i = 0; i < rows.length; i++) {
    if (best < 0 || Math.abs(rows[i]! - expected) <= Math.abs(rows[best]! - expected)) best = i;
  }
  return best;
}
