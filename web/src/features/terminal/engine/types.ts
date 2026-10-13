// What the terminal lens and its key bar agree on.

export type Modifier = "ctrl" | "alt" | null;

// SizeMode: fit resizes the shared pane to this screen, keep attaches at the pane's own grid.
// herdr follows the smallest attached client, so fit on a phone squeezes the desktop's view.
export type SizeMode = "fit" | "keep";

// TermApi is what the key bar needs from a live terminal: type a sequence as if from the
// keyboard, and whether DECCKM (application cursor keys) is on.
export type TermApi = {
  input: (seq: string) => void;
  appCursor: () => boolean;
};
