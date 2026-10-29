// The plain text of a Markdown table cell: what the person reads once the inline marks are rendered, so a copy of the
// cell gives the words and not the asterisks. INLINE is the one grammar renderInline draws and this strips.

export const INLINE = new RegExp(
  [
    "(`+)([\\s\\S]*?[^`])\\1(?!`)", // 1,2 code span
    "\\[([^\\]\\n]+)\\]\\((https?:\\/\\/[^\\s)]+)\\)", // 3,4 link
    "\\*\\*([^*\\n]+?)\\*\\*", // 5 bold
    "__([^_\\n]+?)__", // 6 bold
    "(?<![\\w*])\\*([^*\\n]+?)\\*(?![\\w*])", // 7 italic
    "(?<!\\w)_([^_\\n]+?)_(?!\\w)", // 8 italic
    "~~([^~\\n]+?)~~", // 9 strike
    "(https?:\\/\\/[^\\s<>\"'\\)\\]]+)", // 10 bare url
  ].join("|"),
  "g",
);

const BREAK = /<br\s*\/?>/gi;

// cellText strips the inline marks of one cell, keeps the words of a link and turns a <br> into a line break
export function cellText(source: string): string {
  let out = "";
  let at = 0;
  for (const m of source.matchAll(INLINE)) {
    const idx = m.index ?? 0;
    out += source.slice(at, idx);
    if (m[2] !== undefined) out += m[2];
    else if (m[3] !== undefined) out += cellText(m[3]);
    else if (m[5] !== undefined || m[6] !== undefined) out += cellText((m[5] ?? m[6])!);
    else if (m[7] !== undefined || m[8] !== undefined) out += cellText((m[7] ?? m[8])!);
    else if (m[9] !== undefined) out += cellText(m[9]);
    else out += m[0];
    at = idx + m[0].length;
  }
  out += source.slice(at);
  return out.replace(BREAK, "\n").trim();
}
