// The find over the scrollback, the page's side: remuda-core's attach paints up to XTERM_SCROLLBACK rows into the
// page's xterm, whose search addon finds the query there; the server finds it in the older rows one read at a time
// (/api/scrollback), and the page says how many matches only those hold.

// SCROLLBACK_ROWS is how many rows one find reads: a build log's tail, not the whole history.
export const SCROLLBACK_ROWS = 5000;

// XTERM_SCROLLBACK is how many rows above the screen the page's xterm keeps; the attach paints up to as many
// (attachHistory in internal/core/viewers.go), so the find reads the terminal's own rows.
export const XTERM_SCROLLBACK = 2000;

export type ScrollbackMatch = { row: number; col: number; len: number };
export type Found = { matches: ScrollbackMatch[]; rows: number; truncated: boolean };

// OlderRead is the one read, for a pane and query, of the rows older than a terminal on remuda-core holds for its
// screen: every step of the query awaits it rather than reading again or skipping it, and it is told once.
export type OlderRead = { key: string; found: Promise<Found>; matches: Promise<number>; told: boolean };

// olderRead keeps the read held while the key is the same, and starts one for a new key.
export function olderRead(held: OlderRead | null, key: string, read: () => Promise<Found>): OlderRead {
  if (held?.key === key) return held;
  const found = read();
  return { key, found, matches: found.then((f) => f.matches.length), told: false };
}

// tellsOlder says whether the older rows' matches are news: the run asking is still the current one, this read has not
// been told, the older rows hold the query, and the terminal's own rows still hold none.
export function tellsOlder(current: boolean, read: OlderRead, older: number, counted: number): boolean {
  return current && !read.told && older > 0 && counted === 0;
}
