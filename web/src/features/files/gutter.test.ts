import { describe, expect, it } from "vitest";
import { changeKey, changesOf, codeRows, lineCountOf, opens, toggled, wholeFile, type CodeRow } from "./gutter";

const HEAD = "diff --git a/x.ts b/x.ts\nindex 1..2 100644\n--- a/x.ts\n+++ b/x.ts\n";

describe("the changed lines of a file", () => {
  it("are read off -U0 hunks: an addition, a change with the lines it replaced, a removal between two lines", () => {
    const diff = `${HEAD}@@ -3,0 +4,2 @@\n+added one\n+added two\n@@ -10,2 +12,1 @@\n-old a\n-old b\n+new\n@@ -20,3 +21,0 @@\n-gone 1\n-gone 2\n-gone 3\n`;
    expect(changesOf(diff)).toEqual([
      { kind: "added", line: 3, count: 2, oldStart: 4, old: [] },
      { kind: "changed", line: 11, count: 1, oldStart: 10, old: ["old a", "old b"] },
      { kind: "removed", line: 21, count: 0, oldStart: 20, old: ["gone 1", "gone 2", "gone 3"] },
    ]);
  });

  it("are read off the viewer's own diff, with three lines of context around each run", () => {
    const diff = `${HEAD}@@ -1,7 +1,7 @@\n a\n b\n c\n-d\n+D\n e\n-f\n g\n`;
    expect(changesOf(diff)).toEqual([
      { kind: "changed", line: 3, count: 1, oldStart: 4, old: ["d"] },
      { kind: "removed", line: 5, count: 0, oldStart: 6, old: ["f"] },
    ]);
  });

  it("take a removed -- comment for a removed line, and a run removed after one added as a change of its own", () => {
    const diff = `${HEAD}@@ -1,3 +1,3 @@\n--- a comment\n++new\n-x\n+y\n keep\n`;
    const changes = changesOf(diff);
    expect(changes.map((c) => [c.kind, c.line, c.old])).toEqual([
      ["changed", 0, ["-- a comment"]],
      ["changed", 1, ["x"]],
    ]);
  });

  it("are the whole file for a new one, and none for a diff with no hunks", () => {
    expect(wholeFile(3)).toEqual([{ kind: "added", line: 0, count: 3, oldStart: 0, old: [] }]);
    expect(wholeFile(0)).toEqual([]);
    expect(changesOf("Binary files a/x.png and b/x.png differ\n")).toEqual([]);
    expect(changesOf("")).toEqual([]);
  });

  it("skip git's note that a side has no newline at its end", () => {
    const diff = `${HEAD}@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n`;
    expect(changesOf(diff)).toEqual([{ kind: "changed", line: 0, count: 1, oldStart: 1, old: ["a"] }]);
  });
});

const shape = (rows: CodeRow[]) => rows.map((row) => (row.kind === "line" ? `${row.index}${row.change ? row.change.kind[0] : ""}` : row.kind === "old" ? `old${row.number}` : "removed"));

describe("the rows the File tab draws", () => {
  const changes = changesOf(`${HEAD}@@ -2 +2 @@\n-was two\n+two\n@@ -4,2 +3,0 @@\n-four\n-five\n@@ -7,0 +6 @@\n+six\n`);

  it("mark the changed lines and draw a closed removal as a line of its own", () => {
    expect(shape(codeRows(6, changes, new Set()))).toEqual(["0", "1c", "2", "removed", "3", "4", "5a"]);
  });

  it("show an open change's old lines above the new ones and an open removal's in its place", () => {
    const open = toggled(toggled(new Set(), changes[0]!), changes[1]!);
    expect(shape(codeRows(6, changes, open))).toEqual(["0", "old2", "1c", "2", "old4", "old5", "3", "4", "5a"]);
    expect(shape(codeRows(6, changes, toggled(open, changes[0]!)))).toEqual(["0", "1c", "2", "old4", "old5", "3", "4", "5a"]);
  });

  it("put a removal at the end after the last line", () => {
    const end = changesOf(`${HEAD}@@ -3 +2,0 @@\n-three\n`);
    expect(shape(codeRows(2, end, new Set()))).toEqual(["0", "1", "removed"]);
  });

  it("open only what replaced something", () => {
    expect(opens(changes[0])).toBe(true);
    expect(opens(changes[2])).toBe(false);
    expect(opens(undefined)).toBe(false);
    expect(changeKey(changes[0]!)).not.toBe(changeKey(changes[1]!));
  });
});

describe("a text's lines", () => {
  it("end at its last newline", () => {
    expect(lineCountOf("")).toBe(0);
    expect(lineCountOf("a")).toBe(1);
    expect(lineCountOf("a\n")).toBe(1);
    expect(lineCountOf("a\nb")).toBe(2);
    expect(lineCountOf("a\n\n")).toBe(2);
  });
});
