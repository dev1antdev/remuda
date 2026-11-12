// The text searched for, marked on the screen the player draws: every case-folded occurrence in a row becomes a span
// of its own with the mark, the spans' colours kept. A row whose lower-casing changes its length (a dotted İ) is left
// unmarked rather than marked in the wrong place. Pure.

import type { Line } from "../ansi";

// MIN_MARKED is the query's length from which it is marked, as the search starts at two characters
export const MIN_MARKED = 2;

// rangesIn is where the needle stands in the text, left to right, never overlapping
export function rangesIn(text: string, needle: string): [number, number][] {
  const lower = text.toLowerCase();
  if (!needle || lower.length !== text.length) return [];
  const out: [number, number][] = [];
  for (let at = lower.indexOf(needle); at >= 0; at = lower.indexOf(needle, at + needle.length)) out.push([at, at + needle.length]);
  return out;
}

// markLines is the screen with the query marked in every row that holds it
export function markLines(lines: Line[], query: string): Line[] {
  const needle = query.trim().toLowerCase();
  if ([...needle].length < MIN_MARKED) return lines;
  return lines.map((line) => {
    const ranges = rangesIn(line.map((span) => span.text).join(""), needle);
    return ranges.length ? cut(line, ranges) : line;
  });
}

// cut splits the row's spans at the ranges' edges, the pieces inside a range marked
function cut(line: Line, ranges: [number, number][]): Line {
  const out: Line = [];
  let offset = 0;
  for (const span of line) {
    const end = offset + span.text.length;
    let from = offset;
    for (const [start, stop] of ranges) {
      if (stop <= from || start >= end) continue;
      if (start > from) out.push({ ...span, text: span.text.slice(from - offset, start - offset) });
      const to = Math.min(stop, end);
      out.push({ ...span, text: span.text.slice(Math.max(start, from) - offset, to - offset), mark: true });
      from = to;
    }
    if (from < end) out.push({ ...span, text: span.text.slice(from - offset) });
    offset = end;
  }
  return out;
}
