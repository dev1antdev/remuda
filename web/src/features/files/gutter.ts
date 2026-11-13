// The file viewer's changed lines: git's diff of the file against HEAD read as the runs of lines it added, changed and
// removed, by their place in the file as it is now, each with the old lines it replaced; then the rows the File tab
// draws, where an opened change shows its old lines in place above the new ones.

export type ChangeKind = "added" | "changed" | "removed";

// line: the 0-based index of the change's first line in the file now; for a removal, of the line after the removed ones
export type Change = { kind: ChangeKind; line: number; count: number; oldStart: number; old: string[] };

export type CodeRow =
  | { kind: "line"; index: number; change?: Change } // a line of the file, 0-based
  | { kind: "old"; number: number; text: string; change: Change } // a line the change replaced, by its old number
  | { kind: "removed"; change: Change }; // where a closed removal was

const HEADER = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

// changesOf reads a diff of one file with any context: each run of removed lines followed by added ones is a change, added
// lines alone an addition, removed ones alone a removal
export function changesOf(diff: string): Change[] {
  const changes: Change[] = [];
  let inHunk = false;
  let oldLine = 0;
  let newLine = 0;
  let run: { old: string[]; oldStart: number; added: number; newStart: number } | null = null;
  const flush = () => {
    if (!run) return;
    if (run.added > 0) changes.push({ kind: run.old.length ? "changed" : "added", line: run.newStart - 1, count: run.added, oldStart: run.oldStart, old: run.old });
    else if (run.old.length) changes.push({ kind: "removed", line: newLine - 1, count: 0, oldStart: run.oldStart, old: run.old });
    run = null;
  };
  for (const text of diff.split("\n")) {
    const header = HEADER.exec(text);
    if (header) {
      flush();
      inHunk = true;
      const [, a, b, c, d] = header;
      oldLine = b === "0" ? Number(a) + 1 : Number(a); // a hunk of no lines names the line before it
      newLine = d === "0" ? Number(c) + 1 : Number(c);
      continue;
    }
    if (text.startsWith("diff ")) {
      flush();
      inHunk = false;
    }
    if (!inHunk || text.startsWith("\\")) continue;
    if (text.startsWith("-")) {
      run ??= { old: [], oldStart: oldLine, added: 0, newStart: newLine };
      if (run.added > 0) {
        flush();
        run = { old: [], oldStart: oldLine, added: 0, newStart: newLine };
      }
      run.old.push(text.slice(1));
      oldLine++;
    } else if (text.startsWith("+")) {
      run ??= { old: [], oldStart: oldLine, added: 0, newStart: newLine };
      run.added++;
      newLine++;
    } else {
      flush();
      oldLine++;
      newLine++;
    }
  }
  flush();
  return changes;
}

// wholeFile is a new file's one change: every line added
export const wholeFile = (lines: number): Change[] => (lines > 0 ? [{ kind: "added", line: 0, count: lines, oldStart: 0, old: [] }] : []);

export const changeKey = (change: Change): string => `${change.kind}-${change.oldStart}-${change.line}`;

// opens says a change has old lines to show
export const opens = (change: Change | undefined): boolean => !!change && change.old.length > 0;

// codeRows lays the file's lines out with their changes; an open change's old lines come above its first new line, or in
// place of the thin line a closed removal draws
export function codeRows(lineCount: number, changes: Change[], open: ReadonlySet<string>): CodeRow[] {
  const covering = new Map<number, Change>();
  const starting = new Map<number, Change>();
  const removedAt = new Map<number, Change[]>();
  for (const change of changes) {
    if (change.kind === "removed") {
      const at = Math.min(Math.max(change.line, 0), lineCount);
      removedAt.set(at, [...(removedAt.get(at) ?? []), change]);
      continue;
    }
    starting.set(change.line, change);
    for (let i = change.line; i < Math.min(change.line + change.count, lineCount); i++) covering.set(i, change);
  }
  const rows: CodeRow[] = [];
  const oldRows = (change: Change) => change.old.forEach((text, n) => rows.push({ kind: "old", number: change.oldStart + n, text, change }));
  for (let i = 0; i <= lineCount; i++) {
    for (const removal of removedAt.get(i) ?? []) {
      if (open.has(changeKey(removal))) oldRows(removal);
      else rows.push({ kind: "removed", change: removal });
    }
    const start = starting.get(i);
    if (start && open.has(changeKey(start))) oldRows(start);
    if (i < lineCount) rows.push({ kind: "line", index: i, change: covering.get(i) });
  }
  return rows;
}

// toggled opens a change, or closes it when it is open
export function toggled(open: ReadonlySet<string>, change: Change): Set<string> {
  const next = new Set(open);
  const key = changeKey(change);
  if (!next.delete(key)) next.add(key);
  return next;
}

// lineCountOf is how many rows a text draws: a final newline ends the last line rather than starting another
export const lineCountOf = (text: string): number => {
  if (text === "") return 0;
  const lines = text.split("\n").length;
  return text.endsWith("\n") ? lines - 1 : lines;
};
