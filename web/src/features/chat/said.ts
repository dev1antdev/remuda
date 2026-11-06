// What an agent answered last, in one line, for a tile's state line once it has finished: the text its last turn
// ended with (after its last tool call), the Markdown marks stripped, cut at a word. The server's internal/said
// does the same for the roster.

import type { Turn } from "./api";

export const SAID_MAX = 120;

export function said(turns: Turn[]): string {
  const last = turns[turns.length - 1];
  if (!last || last.role !== "assistant") return "";
  let texts: string[] = [];
  for (const part of last.parts) {
    if (part.type === "tool") texts = [];
    else if (part.type === "text" && part.text?.trim()) texts.push(part.text.trim());
  }
  return firstLine(texts.join("\n\n"));
}

// LEAD_MARKS is the run of list and quote marks and headings a line starts with, taken in one pass
const LEAD_MARKS = /^(?:(?:#{1,6}\s+|>\s*|[-*+]\s+|\d+[.)]\s+|\[[ xX]\]\s+)\s*)+/;
const RULE = /^(?:-{3,}|\*{3,}|_{3,})$/;
const TABLE_RULE = /^\|?[\s:|-]*-[\s:|-]*$/;

// cells reads a table row as its cells joined by " · ", any other line as it is
const cells = (line: string) =>
  line.startsWith("|")
    ? line
        .split("|")
        .map((cell) => cell.trim())
        .filter(Boolean)
        .join(" · ")
    : line;

// firstLine is the first line that says something, its marks stripped and cut to SAID_MAX characters.
export function firstLine(text: string): string {
  let inFence = false;
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (line.startsWith("```") || line.startsWith("~~~")) {
      inFence = !inFence;
      continue;
    }
    if (inFence || RULE.test(line) || (line.startsWith("|") && TABLE_RULE.test(line))) continue;
    const said = plain(cells(line));
    if (said) return cut(said, SAID_MAX);
  }
  return "";
}

// plain is one line with its Markdown marks stripped: list and quote marks, headings, emphasis, code, links, tags. A link,
// a picture or a tag ends before the next bracket of its kind, so a line of thousands of them unclosed costs its length,
// not its square.
export function plain(line: string): string {
  return line
    .trim()
    .replace(LEAD_MARKS, "")
    .replace(/!\[([^[\]]*)\]\([^)[\]]*\)/g, "$1")
    .replace(/\[([^[\]]+)\]\([^)[\]]*\)/g, "$1")
    .replace(/\*\*(.+?)\*\*|__(.+?)__/g, "$1$2")
    .replace(/\*(\S(?:[^*]*\S)?)\*/g, "$1")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/~~(.+?)~~/g, "$1")
    .replace(/<[^<>]+>/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

// cut keeps at most max characters, at a word when one sits past the first two thirds, with an ellipsis.
function cut(line: string, max: number): string {
  const chars = Array.from(line);
  if (chars.length <= max) return line;
  const head = chars.slice(0, max).join("");
  const at = head.lastIndexOf(" ");
  const words = at > 0 ? Array.from(head.slice(0, at)).length : 0;
  const end = words > (max * 2) / 3 ? words : max;
  return chars.slice(0, end).join("").replace(/[ ,;:]+$/, "") + "…";
}
