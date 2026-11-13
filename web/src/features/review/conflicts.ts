// A conflicted file as better-changes shows it: only its conflict blocks with a few lines around each, every line at
// its own number in the file, the markers apart; in one column as the file has it, in two yours beside incoming. When
// the file has no markers to show, a sentence says why.

import { t } from "@/lib/i18n";
import type { ConflictRead } from "./api";

export type ConflictKind = "heading" | "context" | "marker" | "yours" | "base" | "incoming";
export type ConflictLine = { kind: ConflictKind; line?: number; text: string };
export type ConflictPair = { left: ConflictLine | null; right: ConflictLine | null };
// ConflictView is the blocks drawn both ways: inline is the file's own order, split a heading or a pair a row
export type ConflictView = { inline: ConflictLine[]; split: (ConflictLine | ConflictPair)[]; count: number };

export const CONTEXT_LINES = 3;

type Numbered = { text: string; line: number };
type Block = { open: Numbered; yours: Numbered[]; baseMarker?: Numbered; base: Numbered[]; divider?: Numbered; incoming: Numbered[]; close?: Numbered; end: number };

const opens = (text: string) => /^<{7}( |$)/.test(text);

// readBlock is one block from its "<<<<<<<": yours, the base git writes with diff3, and incoming; null when it never closes
function readBlock(lines: Numbered[], start: number): Block | null {
  const block: Block = { open: lines[start]!, yours: [], base: [], incoming: [], end: start };
  let section = block.yours;
  for (let i = start + 1; i < lines.length; i++) {
    const entry = lines[i]!;
    if (section === block.yours && /^\|{7}( |$)/.test(entry.text)) {
      block.baseMarker = entry;
      section = block.base;
    } else if (section !== block.incoming && entry.text === "=======") {
      block.divider = entry;
      section = block.incoming;
    } else if (section === block.incoming && /^>{7}( |$)/.test(entry.text)) {
      block.close = entry;
      block.end = i;
      return block;
    } else section.push(entry);
  }
  return null;
}

// conflictView is the file's blocks with CONTEXT_LINES around each, two close blocks sharing the lines between them; null
// when no block is left
export function conflictView(text: string): ConflictView | null {
  const lines = text
    .replace(/\n$/, "")
    .split("\n")
    .map((line, index) => ({ text: line, line: index + 1 }));
  const parts: ({ plain: Numbered[] } | { block: Block })[] = [];
  let plain: Numbered[] = [];
  for (let i = 0; i < lines.length; i++) {
    const block = opens(lines[i]!.text) ? readBlock(lines, i) : null;
    if (!block) {
      plain.push(lines[i]!);
      continue;
    }
    parts.push({ plain }, { block });
    plain = [];
    i = block.end;
  }
  parts.push({ plain });
  const count = parts.filter((part) => "block" in part).length;
  if (count === 0) return null;
  const inline: ConflictLine[] = [];
  const split: (ConflictLine | ConflictPair)[] = [];
  let shownUpTo = 0;
  const context = (entries: Numbered[]) => {
    for (const entry of entries) {
      if (entry.line <= shownUpTo) continue;
      shownUpTo = entry.line;
      const row: ConflictLine = { kind: "context", line: entry.line, text: entry.text };
      inline.push(row);
      split.push({ left: row, right: row });
    }
  };
  const marker = (entry: Numbered): ConflictLine => ({ kind: "marker", line: entry.line, text: entry.text });
  let number = 0;
  parts.forEach((part, index) => {
    if (!("block" in part)) return;
    number++;
    const { open, yours, baseMarker, base, divider, incoming, close } = part.block;
    const before = (parts[index - 1] as { plain: Numbered[] }).plain;
    const after = (parts[index + 1] as { plain: Numbered[] }).plain;
    const heading: ConflictLine = { kind: "heading", text: t("Conflict {n} of {count} · line {line}", { n: number, count, line: open.line }) };
    inline.push(heading);
    split.push(heading);
    context(before.slice(-CONTEXT_LINES));
    inline.push(marker(open), ...yours.map((entry): ConflictLine => ({ kind: "yours", line: entry.line, text: entry.text })));
    if (baseMarker) inline.push(marker(baseMarker), ...base.map((entry): ConflictLine => ({ kind: "base", line: entry.line, text: entry.text })));
    inline.push(marker(divider!), ...incoming.map((entry): ConflictLine => ({ kind: "incoming", line: entry.line, text: entry.text })), marker(close!));
    split.push({ left: marker(open), right: marker(close!) });
    for (let i = 0; i < Math.max(yours.length, incoming.length); i++) {
      const left = yours[i] ? { kind: "yours" as const, line: yours[i]!.line, text: yours[i]!.text } : null;
      const right = incoming[i] ? { kind: "incoming" as const, line: incoming[i]!.line, text: incoming[i]!.text } : null;
      split.push({ left, right });
    }
    split.push({ left: marker(divider!), right: marker(divider!) });
    shownUpTo = close!.line;
    context(after.slice(0, CONTEXT_LINES));
  });
  return { inline, split, count };
}

export const isPair = (row: ConflictLine | ConflictPair): row is ConflictPair => "left" in row;

// conflictSentence is what stands in for the blocks when there are none to show: a side that deleted the file, a file
// that cannot be read as text, or one already resolved and waiting to be staged
export function conflictSentence(read: ConflictRead): string | null {
  if (read.stages && !read.stages.includes("2")) return t("Conflict: deleted on your side, changed on the incoming side.");
  if (read.stages && !read.stages.includes("3")) return t("Conflict: changed on your side, deleted on the incoming side.");
  if (read.missing) return t("Conflict: the file is not in the folder any more.");
  if (read.binary) return t("Conflict: binary file, no text diff.");
  if (!conflictView(read.text)) return t("No conflict markers left. Stage the file to mark it resolved.");
  return null;
}
