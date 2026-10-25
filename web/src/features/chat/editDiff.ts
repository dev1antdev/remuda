// The change an Edit made, read from the call's own input: old_string against new_string line by line, each removed run
// paired with the added run that follows it and the words that differ marked, the way the Changes lens marks them.

import { changedSpan } from "@/features/review/hunks";
import { highlightLines, type Token } from "@/lib/highlight";

export type Edit = { old: string; new: string; replaceAll: boolean };

// EditRow is one line of the change: context, removed, added, a note (git's "no newline" line), or the head of a further edit
export type EditRow = {
  kind: "ctx" | "del" | "add" | "meta" | "hunk";
  text: string;
  mark?: { head: number; tail: number } | null; // the span that differs between a removed line and its replacement
};

export const NO_NEWLINE = "\\ No newline at end of file";

// editsOf lists the edits an Edit or MultiEdit input carries; a string (an input the server cut) or another tool's is none
export function editsOf(toolName: string | undefined, input: unknown): Edit[] {
  if (!input || typeof input !== "object") return [];
  const obj = input as Record<string, unknown>;
  if (toolName === "Edit") {
    const one = editOf(obj);
    return one ? [one] : [];
  }
  if (toolName === "MultiEdit" && Array.isArray(obj.edits)) {
    return obj.edits.map((e) => (e && typeof e === "object" ? editOf(e as Record<string, unknown>) : null)).filter((e): e is Edit => e !== null);
  }
  return [];
}

function editOf(obj: Record<string, unknown>): Edit | null {
  if (typeof obj.old_string !== "string" || typeof obj.new_string !== "string") return null;
  return { old: obj.old_string, new: obj.new_string, replaceAll: obj.replace_all === true };
}

// editRows is the line diff of one change: unchanged lines as context, removed and added lines run by run, a removed line
// and the added one at its place in the run marked where they differ; a trailing newline only one side has is noted as git does
export function editRows(oldText: string, newText: string): EditRow[] {
  const oldEnds = oldText.endsWith("\n");
  const newEnds = newText.endsWith("\n");
  const noteOld = oldText !== "" && !oldEnds && newEnds;
  const noteNew = newText !== "" && !newEnds && oldEnds;
  const a = linesOf(oldText);
  const b = linesOf(newText);
  const ops = diffLines(noteOld ? withLastMarked(a) : a, noteNew ? withLastMarked(b) : b);
  const rows: EditRow[] = [];
  let i = 0;
  let j = 0;
  let k = 0;
  while (k < ops.length) {
    if (ops[k] === "=") {
      rows.push({ kind: "ctx", text: a[i]! });
      i++;
      j++;
      k++;
      continue;
    }
    const removed: string[] = [];
    const added: string[] = [];
    while (k < ops.length && ops[k] !== "=") {
      if (ops[k] === "-") removed.push(a[i++]!);
      else added.push(b[j++]!);
      k++;
    }
    const pairs = Math.min(removed.length, added.length);
    removed.forEach((text, at) => rows.push({ kind: "del", text, mark: at < pairs ? changedSpan(text, added[at]!) : null }));
    if (noteOld && i === a.length) rows.push({ kind: "meta", text: NO_NEWLINE });
    added.forEach((text, at) => rows.push({ kind: "add", text, mark: at < pairs ? changedSpan(removed[at]!, text) : null }));
    if (noteNew && j === b.length) rows.push({ kind: "meta", text: NO_NEWLINE });
  }
  return rows;
}

// foldRows keeps the first max rows and counts the rest
export function foldRows(rows: EditRow[], max: number): { shown: EditRow[]; more: number } {
  return rows.length <= max ? { shown: rows, more: 0 } : { shown: rows.slice(0, max), more: rows.length - max };
}

// linesOf splits a text into its lines, a trailing newline closing the last line rather than opening an empty one
function linesOf(text: string): string[] {
  if (text === "") return [];
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") lines.pop();
  return lines;
}

// withLastMarked makes the last line unequal to any other, so a line that lost or gained its newline is a change
function withLastMarked(lines: string[]): string[] {
  const out = lines.slice();
  out[out.length - 1] = out[out.length - 1]! + "\u0000";
  return out;
}

const MAX_CELLS = 400_000;

// diffLines is the edit script between two line lists — "=" keep, "-" remove, "+" add — by the longest common
// subsequence; two lists too long to table are read as a whole replacement
function diffLines(a: string[], b: string[]): string[] {
  const n = a.length;
  const m = b.length;
  if (n === 0 || m === 0 || n * m > MAX_CELLS) return [...Array<string>(n).fill("-"), ...Array<string>(m).fill("+")];
  const width = m + 1;
  const table = new Uint32Array((n + 1) * width);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      table[i * width + j] = a[i] === b[j] ? table[(i + 1) * width + j + 1]! + 1 : Math.max(table[(i + 1) * width + j]!, table[i * width + j + 1]!);
    }
  }
  const ops: string[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      ops.push("=");
      i++;
      j++;
    } else if (table[(i + 1) * width + j]! >= table[i * width + j + 1]!) {
      ops.push("-");
      i++;
    } else {
      ops.push("+");
      j++;
    }
  }
  while (i < n) {
    ops.push("-");
    i++;
  }
  while (j < m) {
    ops.push("+");
    j++;
  }
  return ops;
}

// rowTokens colours each side of the change as one text in the file's language, so a string or comment spanning lines keeps
// its kind; a removed line and its replacement sit on different sides, the context on both
export function rowTokens(rows: EditRow[], lang: string): Map<EditRow, Token[]> {
  const tokens = new Map<EditRow, Token[]>();
  if (!lang) return tokens;
  const side = (keep: (row: EditRow) => boolean) => {
    const kept = rows.filter(keep);
    const lines = highlightLines(kept.map((row) => row.text).join("\n"), lang);
    kept.forEach((row, i) => tokens.set(row, lines[i] ?? []));
  };
  side((row) => row.kind === "del" || row.kind === "ctx");
  side((row) => row.kind === "add" || row.kind === "ctx");
  return tokens;
}
