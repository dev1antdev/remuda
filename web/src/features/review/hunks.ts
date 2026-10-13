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
