// highlight marks the query's occurrences in a snippet; none when lower-casing moved the offsets (a dotted İ grows),
// the snippet then shows plain. The conversations' hits and the screens' share it.

import type { ReactElement } from "react";

export function highlight(text: string, q: string) {
  const lower = text.toLowerCase();
  const needle = q.toLowerCase();
  if (!needle || lower.length !== text.length) return text;
  const out: (string | ReactElement)[] = [];
  let i = 0;
  let k = 0;
  for (;;) {
    const j = lower.indexOf(needle, i);
    if (j < 0) break;
    out.push(text.slice(i, j), <mark key={k++}>{text.slice(j, j + needle.length)}</mark>);
    i = j + needle.length;
  }
  out.push(text.slice(i));
  return out;
}
