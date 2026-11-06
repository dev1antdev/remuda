// The lines around a hunk, unfolded from the working-tree file. The new side of every hunk is that file, so the gap
// between two hunks — or before the first, or after the last — is the file's own lines at those numbers, and the old
// side's numbers sit at a fixed offset from them: what the hunks above added less what they removed.

import { rowsOf, type DiffHunk, type DiffRow } from "./hunks";
import { t } from "@/lib/i18n";

export const UNFOLD_STEP = 10;

// Gap is an unchanged stretch of the file: its new-side lines from..to (1-based, inclusive), the old side at new + offset
export type Gap = { from: number; to: number; offset: number };

export type Unfold = { above: number; below: number }; // how many lines of the gaps a hunk has unfolded on each side

export const gapLength = (gap: Gap): number => Math.max(0, gap.to - gap.from + 1);

// span is the first line after a hunk on each side
export function span(hunk: DiffHunk): { newEnd: number; oldEnd: number } {
  let newCount = 0;
  let oldCount = 0;
  for (const row of rowsOf(hunk)) {
    if (row.kind === "add" || row.kind === "") newCount++;
    if (row.kind === "del" || row.kind === "") oldCount++;
  }
  return { newEnd: hunk.newStart + newCount, oldEnd: hunk.oldStart + oldCount };
}

// gapAbove is the stretch between the previous hunk's end (or the file's first line) and this hunk
export function gapAbove(hunk: DiffHunk, prev: DiffHunk | null): Gap {
  return { from: prev ? span(prev).newEnd : 1, to: hunk.newStart - 1, offset: hunk.oldStart - hunk.newStart };
}

// gapBelow is the stretch from a hunk's end to the next hunk, or to the file's last line — null while the file's length is unknown
export function gapBelow(hunk: DiffHunk, next: DiffHunk | null, lineCount: number | null): Gap | null {
  const { newEnd, oldEnd } = span(hunk);
  const to = next ? next.newStart - 1 : lineCount;
  return to === null ? null : { from: newEnd, to, offset: oldEnd - newEnd };
}

// unfolded is the part of a gap on screen, as context rows: the lines nearest the hunk — the gap's bottom for the lines
// above it, its top for the lines below — as many as were asked for and the gap holds; a file shorter than the diff
// says has changed under the review, and the rows stop where it ends
export function unfolded(gap: Gap, lines: string[], shown: number, side: "above" | "below"): DiffRow[] {
  const count = Math.min(shown, gapLength(gap));
  const start = side === "above" ? gap.to - count + 1 : gap.from;
  const rows: DiffRow[] = [];
  for (let line = start; line < start + count; line++) {
    const text = lines[line - 1];
    if (text === undefined) break;
    rows.push({ text: ` ${text}`, kind: "", newLine: line, oldLine: line + gap.offset });
  }
  return rows;
}

// surround is what a hunk shows beyond its own rows: the unfolded lines above it and, for the last hunk, below it
export function surround(hunk: DiffHunk, prev: DiffHunk | null, next: DiffHunk | null, unfold: Unfold | undefined, lines: string[] | null): { above: DiffRow[]; below: DiffRow[] } {
  if (!unfold || !lines) return { above: [], below: [] };
  const below = next ? null : gapBelow(hunk, null, lines.length);
  return {
    above: unfold.above > 0 ? unfolded(gapAbove(hunk, prev), lines, unfold.above, "above") : [],
    below: below && unfold.below > 0 ? unfolded(below, lines, unfold.below, "below") : [],
  };
}

// fileLines cuts a file's text into its lines, the newline that ends the last one not counted as a line of its own
export function fileLines(text: string): string[] {
  const lines = text.split("\n");
  if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

// whyNotUnfold says why a file's lines are not to be had: the viewer cut it at its cap, so the lines past that are missing
export function whyNotUnfold(read: { truncated: boolean; max: number }): string | null {
  return read.truncated ? t("The file is over {kb} KB and remuda reads only its start, so the lines around the hunks cannot be unfolded", { kb: Math.round(read.max / 1024) }) : null;
}
