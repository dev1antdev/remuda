import { describe, expect, it } from "vitest";
import { changedSpan, pairRows, partners, rowsOf, splitHunks } from "./hunks";

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

describe("pairRows", () => {
  const rows = rowsOf(splitHunks(["@@ -1,4 +1,5 @@", " same", "-gone", "-gone too", "+came", "+came too", "+came last", " end", ""].join("\n"))[0]!);
  it("puts a context row on both sides and each removed row beside the added one that replaced it", () => {
    const pairs = pairRows(rows).map((pair) => [pair.old?.text ?? null, pair.new?.text ?? null]);
    expect(pairs).toEqual([
      [" same", " same"],
      ["-gone", "+came"],
      ["-gone too", "+came too"],
      [null, "+came last"],
      [" end", " end"],
    ]);
  });
  it("leaves an addition without a removal alone on the new side, and the other way round", () => {
    const only = rowsOf(splitHunks(["@@ -1,2 +1,2 @@", "+new", " x", "-old", ""].join("\n"))[0]!);
    expect(pairRows(only).map((pair) => [pair.old?.text ?? null, pair.new?.text ?? null])).toEqual([
      [null, "+new"],
      [" x", " x"],
      ["-old", null],
    ]);
  });
  it("names the partners of a replaced row both ways, never a context row", () => {
    const map = partners(pairRows(rows));
    expect(map.get(rows[1]!)).toBe(rows[3]);
    expect(map.get(rows[3]!)).toBe(rows[1]);
    expect(map.has(rows[0]!)).toBe(false);
    expect(map.has(rows[5]!)).toBe(false);
  });
});

describe("pairRows with a missing final newline", () => {
  it("keeps the removed line beside its replacement and lays the two notes side by side", () => {
    const text = ["@@ -1 +1 @@", "-old", "\\ No newline at end of file", "+new", "\\ No newline at end of file"].join("\n");
    const pairs = pairRows(rowsOf(splitHunks(text)[0]!));
    expect(pairs.map((pair) => [pair.old?.text ?? null, pair.new?.text ?? null])).toEqual([
      ["-old", "+new"],
      ["\\ No newline at end of file", "\\ No newline at end of file"],
    ]);
    expect(partners(pairs).size).toBe(2);
  });
});

describe("changedSpan", () => {
  it("marks an emoji whole, never half a surrogate pair", () => {
    expect(changedSpan("a😀b", "a😁b")).toEqual({ head: 1, tail: 1 });
  });
  it("is the head and tail two lines share, out to the word boundaries", () => {
    expect(changedSpan("    foo(a, b)", "    bar(a, b)")).toEqual({ head: 4, tail: 6 });
    expect(changedSpan("abc", "abc def")).toEqual({ head: 3, tail: 0 });
    expect(changedSpan("const total = sum + 1;", "const total = sum + 10;")).toEqual({ head: 20, tail: 1 });
  });
  it("is nothing when the lines share nothing but a piece of a word, or everything", () => {
    expect(changedSpan("alpha", "beta")).toBeNull();
    expect(changedSpan("abc", "abcdef")).toBeNull();
    expect(changedSpan("same", "same")).toBeNull();
  });
});
