// The split view: the chat beside the terminal on a wide screen with a mouse. The pref `split` asks for it; the page
// grants it only where there is room, and the focused lens — the store's `lens` — says which side the tab strip's
// tools, the find and the keys serve. The Changes lens stays a lens of its own and replaces both.

import type { Lens } from "./store";

export const SPLIT_MIN_WIDTH = 1180; // px: narrower, the two lenses would starve each other
export const SPLIT_QUERY = `(min-width: ${SPLIT_MIN_WIDTH}px)`;

// canSplit says whether this screen may show the two side by side
export function canSplit(wide: boolean, touch: boolean): boolean {
  return wide && !touch;
}

// splitOn is whether the page shows the split right now: asked for, possible, and no Changes lens in the way
export function splitOn(asked: boolean, wide: boolean, touch: boolean, lens: Lens): boolean {
  return asked && canSplit(wide, touch) && lens !== "changes";
}

// shownLenses lists the lenses on screen: both sides in the split, else the one chosen
export function shownLenses(lens: Lens, split: boolean): Lens[] {
  return split ? ["chat", "terminal"] : [lens];
}
