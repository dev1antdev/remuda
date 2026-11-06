// The emoji picker's pure part: finding an emoji by its name, and the ones picked lately.

import type { EmojiGroup } from "./emojiData";

const words = (text: string) => text.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean);

// searchEmoji finds the emoji whose name has a word starting with every word typed, case-folded, in the groups'
// order; an emoji typed as itself finds itself
export function searchEmoji(groups: EmojiGroup[], query: string, limit = 120): [string, string][] {
  const typed = query.trim();
  if (!typed) return [];
  const wanted = words(typed);
  const found: [string, string][] = [];
  for (const group of groups) {
    for (const entry of group.emoji) {
      const [emoji, name] = entry;
      const nameWords = words(name);
      const hit = emoji === typed || (wanted.length > 0 && wanted.every((w) => nameWords.some((n) => n.startsWith(w))));
      if (!hit) continue;
      found.push(entry);
      if (found.length >= limit) return found;
    }
  }
  return found;
}

// pushRecent puts an emoji first among the recent ones, once, keeping at most max
export function pushRecent(recent: string[], emoji: string, max = 24): string[] {
  return [emoji, ...recent.filter((e) => e !== emoji)].slice(0, max);
}
