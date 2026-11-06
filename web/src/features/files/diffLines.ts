// The file viewer's Changes tab: git's diff of one file read line by line — the header, the hunks, the lines added,
// removed and kept — with the code of each hunk coloured in the file's language, the old side (removed and kept lines)
// and the new side (added and kept lines) each tokenized as one text, so a string or a comment spanning lines keeps
// its kind on every line.

import { highlightLines, type Token } from "@/lib/highlight";

export type DiffLineKind = "add" | "del" | "hunk" | "meta" | "context" | "note";
export type DiffLine = { kind: DiffLineKind; text: string; tokens?: Token[] };

// diffLines reads a diff; a line starting with --- or +++ is the header only before a file's first hunk, so a removed
// "-- comment" is a removed line
export function diffLines(diff: string, lang = ""): DiffLine[] {
  const texts = diff.split("\n");
  if (texts[texts.length - 1] === "") texts.pop();
  let inHunk = false;
  const lines = texts.map((text): DiffLine => {
    if (text.startsWith("diff ")) inHunk = false;
    if (text.startsWith("@@")) {
      inHunk = true;
      return { kind: "hunk", text };
    }
    if (!inHunk) return { kind: "meta", text };
    if (text.startsWith("+")) return { kind: "add", text };
    if (text.startsWith("-")) return { kind: "del", text };
    if (text.startsWith("\\")) return { kind: "note", text };
    return { kind: "context", text };
  });
  if (lang) colour(lines, lang);
  return lines;
}

// colour gives every code line of each hunk its tokens: the old side first, then the new, a kept line ending with the new side's
function colour(lines: DiffLine[], lang: string) {
  let start = 0;
  while (start < lines.length) {
    if (lines[start]!.kind !== "hunk") {
      start++;
      continue;
    }
    let end = start + 1;
    while (end < lines.length && lines[end]!.kind !== "hunk" && lines[end]!.kind !== "meta") end++;
    const hunk = lines.slice(start + 1, end);
    const side = (keep: (line: DiffLine) => boolean) => {
      const kept = hunk.filter(keep);
      const tokens = highlightLines(kept.map((line) => line.text.slice(1)).join("\n"), lang);
      kept.forEach((line, i) => (line.tokens = tokens[i] ?? []));
    };
    side((line) => line.kind === "del" || line.kind === "context");
    side((line) => line.kind === "add" || line.kind === "context");
    start = end;
  }
}

// changeCounts is the lines added and removed, for the tab's badge
export function changeCounts(lines: DiffLine[]): { add: number; del: number } {
  let add = 0;
  let del = 0;
  for (const line of lines) {
    if (line.kind === "add") add++;
    else if (line.kind === "del") del++;
  }
  return { add, del };
}
