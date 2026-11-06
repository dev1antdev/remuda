// Finding in the conversation shown: where a query occurs in a text, and the walk from match to match.

import { t } from "@/lib/i18n";

export type Span = [start: number, end: number];

// MAX_MATCHES bounds what one find paints: past it the count reads "2000+" and the rest is left unmarked.
export const MAX_MATCHES = 2000;

// findRanges lists every occurrence of query in text, case-folded, left to right and never overlapping.
export function findRanges(text: string, query: string): Span[] {
  if (!query) return [];
  const needle = query.toLowerCase();
  const hay = text.toLowerCase();
  const out: Span[] = [];
  if (hay.length === text.length && needle.length === query.length) {
    for (let at = hay.indexOf(needle); at >= 0; at = hay.indexOf(needle, at + needle.length)) out.push([at, at + needle.length]);
    return out;
  }
  // folding moved the offsets (İ, ß): compare piece by piece on the text as it is
  for (let at = 0; at + query.length <= text.length; at++) {
    if (text.slice(at, at + query.length).toLowerCase() !== needle) continue;
    out.push([at, at + query.length]);
    at += query.length - 1;
  }
  return out;
}

// step moves the current match by one and wraps at either end; with nothing to step through it stays at 0.
export function step(at: number, total: number, by: 1 | -1): number {
  if (total <= 0) return 0;
  return (at + by + total) % total;
}

// clamp keeps the current match inside a list that may have shrunk under it.
export function clamp(at: number, total: number): number {
  return total <= 0 ? 0 : Math.min(Math.max(at, 0), total - 1);
}

// FindView is what the current match's place hangs on: the query, the match chosen, and whether the log has all its turns
export type FindView = { query: string; at: number; mounted: boolean };

// revealDue says whether to bring the current match into view: the find just opened, a new query or step, or the log
// just finished mounting its earlier turns, which numbers the matches afresh from the start; a poll moves nothing
export function revealDue(was: FindView | null, now: FindView): boolean {
  if (!was) return true;
  return was.query !== now.query || was.at !== now.at || (!was.mounted && now.mounted);
}

// countWords says "3 of 12", "0 of 0" while nothing matches, and "2000+" past the bound, in the page's language.
export function countWords(at: number, total: number): string {
  if (total <= 0) return t("0 of 0");
  return t("{at} of {total}", { at: at + 1, total: total >= MAX_MATCHES ? `${MAX_MATCHES}+` : total });
}
