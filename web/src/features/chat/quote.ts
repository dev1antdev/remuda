// A piece of an agent's answer quoted into the reply: the selected text as a Markdown quote after what is already
// typed, and the key that asks for it. The pure part of useQuote and the composer's insertion.

import { FIELDS, type KeyPress } from "@/app/nextKey";

// QUOTE_MAX is how much of a selection a quote keeps; a longer one is cut at a line with a note
export const QUOTE_MAX = 4000;

// quoteText is the selection as a Markdown quote: every line behind "> ", the blank lines at its ends dropped, the
// trailing spaces of each line too; "" for a selection of nothing but blanks
export function quoteText(selected: string): string {
  const lines = selected.replace(/\r\n?/g, "\n").split("\n").map((line) => line.replace(/\s+$/, ""));
  while (lines.length && !lines[0]) lines.shift();
  while (lines.length && !lines[lines.length - 1]) lines.pop();
  if (!lines.length) return "";
  let cut = false;
  let size = 0;
  const kept: string[] = [];
  for (const line of lines) {
    size += line.length + 1;
    if (size > QUOTE_MAX && kept.length) {
      cut = true;
      break;
    }
    if (size > QUOTE_MAX) {
      kept.push(cutLine(line, QUOTE_MAX));
      cut = true;
      break;
    }
    kept.push(line);
  }
  const quoted = kept.map((line) => (line ? `> ${line}` : ">"));
  if (cut) quoted.push("> …");
  return quoted.join("\n");
}

// cutLine is a first line too long to quote whole, cut at the last space before max, else at max itself, never between
// the halves of a surrogate pair
function cutLine(line: string, max: number): string {
  const space = line.lastIndexOf(" ", max);
  let end = space > max / 2 ? space : max;
  const code = line.charCodeAt(end - 1);
  if (code >= 0xd800 && code <= 0xdbff) end -= 1;
  return line.slice(0, end).replace(/\s+$/, "");
}

// withQuote puts the quote after what is already typed, a blank line on each side, and the caret where the reply goes
export function withQuote(draft: string, quote: string): { draft: string; caret: number } {
  if (!quote) return { draft, caret: draft.length };
  const before = draft.replace(/\s+$/, "");
  const next = `${before ? `${before}\n\n` : ""}${quote}\n\n`;
  return { draft: next, caret: next.length };
}

// quoteKeyAsked says whether a key press quotes the selection: Q alone, by key code so any layout works, while text in
// an answer is selected and the focus is in no field, which keeps its letters
export function quoteKeyAsked(press: KeyPress, selected: boolean): boolean {
  if (!selected || press.code !== "KeyQ" || press.altKey || press.metaKey || press.ctrlKey || press.shiftKey) return false;
  return !press.target?.closest?.(FIELDS);
}
