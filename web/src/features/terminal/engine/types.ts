// What the terminal lens and its key bar agree on.

import type { Find } from "@/app/store";

export type Modifier = "ctrl" | "alt" | null;

// SizeMode: fit resizes the shared pane to this screen, keep attaches at the pane's own grid.
// herdr follows the smallest attached client, so fit on a phone squeezes the desktop's view.
export type SizeMode = "fit" | "keep";

// SearchApi is the find's hold on the screen: the match at a row of it (near a character column) marked once the
// repaint lands, as long as the query is on that row, with every other match on the screen decorated, and the marks cleared.
// find, where the terminal holds its own rows (a live terminal on remuda-core), looks through them itself: a new
// query (step 0) from the top, then the next or previous match, scrolled into view and marked; hear is told which one
// it is of how many, then again whenever output arriving changes the count, until the next find or the marks clear.
export type FoundCount = Pick<Find, "total" | "at">;
export type SearchApi = {
  show: (query: string, expectedRow: number, col: number) => void;
  find?: (query: string, step: 0 | 1 | -1, hear: (found: FoundCount) => void) => void;
  clear: () => void;
};

// TermApi is what the key bar and the find bar need from a live terminal: type a sequence as if from the
// keyboard, whether DECCKM (application cursor keys) is on, the focus, and the find over the scrollback.
export type TermApi = {
  input: (seq: string) => void;
  appCursor: () => boolean;
  focus: () => void;
  search: SearchApi;
};
