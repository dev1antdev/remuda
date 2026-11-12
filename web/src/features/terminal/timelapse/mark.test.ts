import { describe, expect, it } from "vitest";
import { parseAnsi } from "../ansi";
import { markLines, rangesIn } from "./mark";

const marked = (lines: ReturnType<typeof parseAnsi>) => lines.map((line) => line.filter((span) => span.mark).map((span) => span.text));

describe("the text found, marked on the screen", () => {
  it("finds every case-folded occurrence, left to right, never overlapping", () => {
    expect(rangesIn("Failed, then FAILED again", "failed")).toEqual([
      [0, 6],
      [13, 19],
    ]);
    expect(rangesIn("aaaa", "aa")).toEqual([
      [0, 2],
      [2, 4],
    ]);
    expect(rangesIn("İstanbul failed", "failed")).toEqual([]); // lowering moved the offsets: no mark rather than a wrong one
  });

  it("splits the spans at the match's edges and keeps their colours", () => {
    const lines = markLines(parseAnsi("\x1b[31mMigra\x1b[1mtion\x1b[0m failed\nnothing here\nmigration FAILED"), "migration failed");
    expect(marked(lines)).toEqual([["Migra", "tion", " failed"], [], ["migration FAILED"]]);
    const first = lines[0]!;
    expect(first.map((span) => span.text).join("")).toBe("Migration failed");
    expect(first[0]!.fg).toBeTruthy();
    expect(first[1]!.bold).toBe(true);
    expect(first[2]!.fg).toBeUndefined();
  });

  it("marks inside a span with text either side", () => {
    const [line] = markLines(parseAnsi("⏺ Error: disk full (exit 1)"), "disk full");
    expect(line!.map((span) => [span.text, !!span.mark])).toEqual([
      ["⏺ Error: ", false],
      ["disk full", true],
      [" (exit 1)", false],
    ]);
  });

  it("leaves the screen as it is for a query too short to search", () => {
    const lines = parseAnsi("a b c");
    expect(markLines(lines, "a")).toBe(lines);
    expect(markLines(lines, "  ")).toBe(lines);
  });
});
