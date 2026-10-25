import { describe, expect, it } from "vitest";
import { fileLines, gapAbove, gapBelow, gapLength, span, surround, unfolded, whyNotUnfold } from "./context";
import { splitHunks } from "./hunks";

// a 30-line file "l1".."l30": two lines inserted after l5, l15 replaced, l28 removed
const DIFF = `--- a/f
+++ b/f
@@ -3,6 +3,8 @@
 l3
 l4
 l5
+new a
+new b
 l6
 l7
 l8
@@ -12,7 +14,7 @@
 l12
 l13
 l14
-l15
+l15 changed
 l16
 l17
 l18
@@ -25,6 +27,5 @@
 l25
 l26
 l27
-l28
 l29
 l30
`;
const after = [...Array.from({ length: 5 }, (_, i) => `l${i + 1}`), "new a", "new b", ...Array.from({ length: 9 }, (_, i) => `l${i + 6}`), "l15 changed", ...Array.from({ length: 12 }, (_, i) => `l${i + 16}`), "l29", "l30"];
const hunks = splitHunks(DIFF);
const SHORT = "@@ -3,3 +3,4 @@\n l3\n+x\n l4\n l5\n"; // one hunk in the middle of a file

describe("span", () => {
  it("ends a hunk on each side after what it showed", () => {
    expect(span(hunks[0]!)).toEqual({ newEnd: 11, oldEnd: 9 });
    expect(span(hunks[1]!)).toEqual({ newEnd: 21, oldEnd: 19 });
    expect(span(hunks[2]!)).toEqual({ newEnd: 32, oldEnd: 31 });
  });
});

describe("gapAbove", () => {
  it("runs from the file's first line to the first hunk", () => {
    const gap = gapAbove(hunks[0]!, null);
    expect(gap).toEqual({ from: 1, to: 2, offset: 0 });
    expect(gapLength(gap)).toBe(2);
  });
  it("runs between two hunks with the old side shifted by the lines added above", () => {
    const gap = gapAbove(hunks[1]!, hunks[0]!);
    expect(gap).toEqual({ from: 11, to: 13, offset: -2 });
    expect(gapLength(gap)).toBe(3);
  });
  it("is empty for a hunk at line 1", () => {
    const [first] = splitHunks("@@ -1,3 +1,4 @@\n+top\n a\n b\n c\n");
    expect(gapLength(gapAbove(first!, null))).toBe(0);
  });
});

describe("gapBelow", () => {
  it("is empty after a last hunk that reaches the file's end", () => {
    const gap = gapBelow(hunks[2]!, null, after.length);
    expect(gap).toEqual({ from: 32, to: 31, offset: -1 });
    expect(gapLength(gap!)).toBe(0);
  });
  it("counts the lines after a hunk that stops short of the end", () => {
    const [only] = splitHunks(SHORT);
    expect(gapBelow(only!, null, 30)).toEqual({ from: 7, to: 30, offset: -1 });
  });
  it("is unknown while the file's length is", () => {
    expect(gapBelow(hunks[2]!, null, null)).toBeNull();
  });
  it("stops at the next hunk", () => {
    expect(gapBelow(hunks[0]!, hunks[1]!, after.length)).toEqual({ from: 11, to: 13, offset: -2 });
  });
});

describe("unfolded", () => {
  it("takes the lines nearest the hunk from above, numbered on both sides", () => {
    const rows = unfolded(gapAbove(hunks[1]!, hunks[0]!), after, 2, "above");
    expect(rows).toEqual([
      { text: " l10", kind: "", newLine: 12, oldLine: 10 },
      { text: " l11", kind: "", newLine: 13, oldLine: 11 },
    ]);
  });
  it("never crosses into the neighbour hunk when the gap is shorter than asked", () => {
    const rows = unfolded(gapAbove(hunks[1]!, hunks[0]!), after, 10, "above");
    expect(rows.map((row) => row.newLine)).toEqual([11, 12, 13]);
    expect(rows[0]!.text).toBe(" l9");
  });
  it("takes the whole gap for Infinity", () => {
    expect(unfolded(gapAbove(hunks[0]!, null), after, Infinity, "above").map((row) => row.text)).toEqual([" l1", " l2"]);
  });
  it("takes the lines nearest the hunk from below", () => {
    const [only] = splitHunks(SHORT);
    const rows = unfolded(gapBelow(only!, null, 30)!, after, 3, "below");
    expect(rows.map((row) => [row.newLine, row.oldLine])).toEqual([
      [7, 6],
      [8, 7],
      [9, 8],
    ]);
  });
  it("stops where a file shorter than the diff says ends", () => {
    const [only] = splitHunks(SHORT);
    expect(unfolded(gapBelow(only!, null, 30)!, ["a", "b", "c", "d", "e", "f", "g"], 5, "below")).toHaveLength(1);
  });
});

describe("surround", () => {
  it("shows nothing until something was unfolded and the file read", () => {
    expect(surround(hunks[1]!, hunks[0]!, hunks[2]!, undefined, after)).toEqual({ above: [], below: [] });
    expect(surround(hunks[1]!, hunks[0]!, hunks[2]!, { above: 10, below: 0 }, null)).toEqual({ above: [], below: [] });
  });
  it("unfolds above any hunk and below only the last", () => {
    const [only] = splitHunks(SHORT);
    expect(surround(only!, null, null, { above: 1, below: 1 }, after)).toEqual({
      above: [{ text: " l2", kind: "", newLine: 2, oldLine: 2 }],
      below: [{ text: " new b", kind: "", newLine: 7, oldLine: 6 }],
    });
    expect(surround(hunks[0]!, null, hunks[1]!, { above: 1, below: 1 }, after).below).toEqual([]);
  });
});

describe("fileLines", () => {
  it("drops the newline that ends the last line, and only that", () => {
    expect(fileLines("a\nb\n")).toEqual(["a", "b"]);
    expect(fileLines("a\nb")).toEqual(["a", "b"]);
    expect(fileLines("a\n\n")).toEqual(["a", ""]);
    expect(fileLines("")).toEqual([""]);
  });
});

describe("whyNotUnfold", () => {
  it("names the cap for a file cut at it", () => {
    expect(whyNotUnfold({ truncated: true, max: 512 * 1024 })).toMatch(/over 512 KB/);
    expect(whyNotUnfold({ truncated: false, max: 512 * 1024 })).toBeNull();
  });
});
