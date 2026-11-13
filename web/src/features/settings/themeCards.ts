// The pure part of the theme cards in Settings › Look: which accents a card shows, and where an arrow key moves the pick.

import type { Accent } from "@/lib/theme";

// previewHues are the three accents a theme's card wears: the person's own, then the ones that mean blocked and done
export function previewHues(accent: Accent): Accent[] {
  return [accent, ...(["red", "green", "blue"] as const).filter((a) => a !== accent)].slice(0, 3);
}

// stepChoice is the choice a key moves to in a radio group, as the platform's own radios do — the arrows round and
// round, Home and End to the ends — or null for any other key
export function stepChoice<T>(list: readonly T[], current: T, key: string): T | null {
  const at = Math.max(0, list.indexOf(current));
  const last = list.length - 1;
  switch (key) {
    case "ArrowRight":
    case "ArrowDown":
      return list[at === last ? 0 : at + 1] ?? null;
    case "ArrowLeft":
    case "ArrowUp":
      return list[at === 0 ? last : at - 1] ?? null;
    case "Home":
      return list[0] ?? null;
    case "End":
      return list[last] ?? null;
  }
  return null;
}
