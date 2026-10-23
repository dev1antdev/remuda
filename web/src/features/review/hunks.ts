// A unified diff cut into hunks and rows, each row with its line number in the new file or the old one.

export type DiffHunk = { header: string; oldStart: number; newStart: number; lines: string[] };

export function splitHunks(text: string): DiffHunk[] {
  const hunks: DiffHunk[] = [];
  let current: DiffHunk | null = null;
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") lines.pop(); // the trailing newline
  for (const line of lines) {
    if (line.startsWith("@@")) {
      const range = /-(\d+)(?:,\d+)? \+(\d+)/.exec(line);
      current = { header: line, oldStart: range ? Number(range[1]) : 1, newStart: range ? Number(range[2]) : 1, lines: [] };
      hunks.push(current);
    } else if (current) {
      current.lines.push(line); // the file headers sit before the first @@; "---x" or "+++i" inside a hunk is a row
    }
  }
  return hunks;
}

export type DiffRow = { text: string; kind: "add" | "del" | "meta" | ""; newLine: number | null; oldLine: number | null };

export function rowsOf(hunk: DiffHunk): DiffRow[] {
  let newLine = hunk.newStart;
  let oldLine = hunk.oldStart;
  return hunk.lines.map((text) => {
    const kind = text.startsWith("+") ? "add" : text.startsWith("-") ? "del" : text.startsWith("\\") ? "meta" : "";
    const row: DiffRow = { text, kind, newLine: kind === "del" || kind === "meta" ? null : newLine, oldLine: kind === "add" || kind === "meta" ? null : oldLine };
    if (kind === "add") newLine++;
    else if (kind === "del") oldLine++;
    else if (kind !== "meta") {
      newLine++;
      oldLine++;
    }
    return row;
  });
}

export type SplitRow = { old: DiffRow | null; new: DiffRow | null };

// pairRows lays the rows side by side: a context row on both sides, each removed row beside the added row that follows the run it is in, the rest alone
export function pairRows(rows: DiffRow[]): SplitRow[] {
  const pairs: SplitRow[] = [];
  let index = 0;
  while (index < rows.length) {
    const row = rows[index]!;
    if (row.kind !== "del" && row.kind !== "add") {
      pairs.push({ old: row, new: row });
      index++;
      continue;
    }
    const removed: DiffRow[] = [];
    const added: DiffRow[] = [];
    while (index < rows.length && rows[index]!.kind === "del") removed.push(rows[index++]!);
    const oldNote = removed.length > 0 && rows[index]?.kind === "meta" ? rows[index++]! : null; // "\ No newline at end of file", the old side's
    while (index < rows.length && rows[index]!.kind === "add") added.push(rows[index++]!);
    const newNote = added.length > 0 && rows[index]?.kind === "meta" ? rows[index++]! : null;
    for (let at = 0; at < Math.max(removed.length, added.length); at++) pairs.push({ old: removed[at] ?? null, new: added[at] ?? null });
    if (oldNote || newNote) pairs.push({ old: oldNote, new: newNote });
  }
  return pairs;
}

// partners maps each removed row to the added row that replaced it and back, for the highlight of what changed between them
export function partners(pairs: SplitRow[]): Map<DiffRow, DiffRow> {
  const map = new Map<DiffRow, DiffRow>();
  for (const pair of pairs) {
    if (pair.old && pair.new && pair.old !== pair.new && pair.old.kind !== "meta") {
      map.set(pair.old, pair.new);
      map.set(pair.new, pair.old);
    }
  }
  return map;
}

// changedSpan is where a removed line and its replacement differ: the lengths of the head and tail they share, moved out
// to word boundaries so a word changed in the middle is marked whole; null when they share nothing, or everything
export function changedSpan(oldText: string, newText: string): { head: number; tail: number } | null {
  const max = Math.min(oldText.length, newText.length);
  let head = 0;
  while (head < max && oldText[head] === newText[head]) head++;
  let tail = 0;
  while (tail < max - head && oldText[oldText.length - 1 - tail] === newText[newText.length - 1 - tail]) tail++;
  if (head === oldText.length && oldText.length === newText.length) return null;
  // never between the halves of a surrogate pair: an emoji that changed is marked whole, not shown broken
  if (head > 0 && lowSurrogate(oldText.charCodeAt(head))) head--;
  if (tail > 0 && lowSurrogate(oldText.charCodeAt(oldText.length - tail))) tail--;
  while (head > 0 && wordChar(oldText[head - 1]) && (wordChar(oldText[head]) || wordChar(newText[head]))) head--;
  while (tail > 0 && wordChar(oldText[oldText.length - tail]) && (wordChar(oldText[oldText.length - tail - 1]) || wordChar(newText[newText.length - tail - 1]))) tail--;
  if (head + tail === 0) return null;
  return { head, tail };
}

const wordChar = (char: string | undefined) => char !== undefined && /[\p{L}\p{N}_]/u.test(char);
const lowSurrogate = (code: number) => code >= 0xdc00 && code <= 0xdfff;
