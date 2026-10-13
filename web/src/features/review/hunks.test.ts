import { describe, expect, it } from "vitest";
import { rowsOf, splitHunks } from "./hunks";

const diff = ["diff --git a/f.go b/f.go", "--- a/f.go", "+++ b/f.go", "@@ -10,3 +10,4 @@ func main() {", " context", "-old line", "+new line", "+another", "\\ No newline at end of file", "@@ -40 +41 @@", " tail", ""].join("\n");

describe("splitHunks", () => {
  it("cuts at every @@ and reads the start lines, the file headers left out", () => {
    const hunks = splitHunks(diff);
    expect(hunks.map((h) => [h.oldStart, h.newStart, h.lines.length])).toEqual([
      [10, 10, 5],
      [40, 41, 1],
    ]);
    expect(hunks[0]!.header).toBe("@@ -10,3 +10,4 @@ func main() {");
  });
  it("is empty for a binary or identical file", () => {
    expect(splitHunks("Binary files differ\n")).toEqual([]);
  });
});

describe("rowsOf", () => {
  it("numbers each row in the new file, the old one for a removed row, neither for a note", () => {
    const rows = rowsOf(splitHunks(diff)[0]!).map((r) => [r.kind, r.oldLine, r.newLine]);
    expect(rows).toEqual([
      ["", 10, 10],
      ["del", 11, null],
      ["add", null, 11],
      ["add", null, 12],
      ["meta", null, null],
    ]);
  });
});
