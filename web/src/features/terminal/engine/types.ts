// What the terminal lens and its key bar agree on.

export type Modifier = "ctrl" | "alt" | null;

// SizeMode: fit resizes the shared pane to this screen, keep attaches at the pane's own grid.
// herdr follows the smallest attached client, so fit on a phone squeezes the desktop's view.
export type SizeMode = "fit" | "keep";

// SearchApi is the find's hold on the screen: the match at a row of it (a character column and length) marked once
// the repaint lands, with every other match on the screen decorated, and the marks cleared.
export type SearchApi = {
  show: (query: string, expectedRow: number, col: number, length: number) => void;
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
