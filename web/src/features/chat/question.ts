// The pure part of the question card: the keys that walk the cursor to a choice, and how the dialog's words split.

import type { QuestionOption } from "./api";

// keysTo walks the cursor from the picked choice to the one at index and confirms it.
export function keysTo(options: QuestionOption[], index: number): string[] {
  const picked = Math.max(0, options.findIndex((o) => o.picked));
  const steps = index - picked;
  const keys: string[] = Array.from({ length: Math.abs(steps) }, () => (steps > 0 ? "down" : "up"));
  keys.push("enter");
  return keys;
}

// questionLines splits the dialog's words: the first line heads it, the last line asks, what is between is the detail.
export function questionLines(text: string): { head: string; detail: string; ask: string } {
  const lines = text.split("\n");
  if (lines.length < 2) return { head: "", detail: "", ask: lines[0] ?? "" };
  return { head: lines[0] ?? "", detail: lines.slice(1, -1).filter((l) => l !== "").join("\n"), ask: lines[lines.length - 1] ?? "" };
}
