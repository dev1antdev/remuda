import { describe, expect, it } from "vitest";
import { conflictSentence, conflictView, isPair } from "./conflicts";

const lines = (count: number, from = 1) => Array.from({ length: count }, (_, i) => `line ${from + i}`);
const file = [...lines(10), "<<<<<<< HEAD", "ours", "=======", "theirs", "also theirs", ">>>>>>> feature", ...lines(10, 17), "<<<<<<< HEAD", "ours again", "||||||| base", "the base", "=======", "theirs again", ">>>>>>> feature", "tail"].join("\n");

describe("a conflicted file", () => {
  it("shows only its blocks with three lines around each, at their own numbers", () => {
    const view = conflictView(file)!;
    expect(view.count).toBe(2);
    const inline = view.inline.map((row) => `${row.kind}:${row.line ?? ""}:${row.text}`);
    expect(inline.slice(0, 10)).toEqual([
      "heading::Conflict 1 of 2 · line 11",
      "context:8:line 8",
      "context:9:line 9",
      "context:10:line 10",
      "marker:11:<<<<<<< HEAD",
      "yours:12:ours",
      "marker:13:=======",
      "incoming:14:theirs",
      "incoming:15:also theirs",
      "marker:16:>>>>>>> feature",
    ]);
    expect(inline).toContain("base:30:the base");
    expect(inline).not.toContain("context:20:line 20"); // the plain lines between blocks past the three around each
    expect(inline.at(-1)).toBe("context:34:tail");
  });

  it("puts yours beside incoming in two columns", () => {
    const pairs = conflictView(file)!.split.filter(isPair);
    const yoursRow = pairs.find((pair) => pair.left?.kind === "yours" && pair.left.text === "ours")!;
    expect(yoursRow.right).toEqual({ kind: "incoming", line: 14, text: "theirs" });
    expect(pairs.some((pair) => pair.left === null && pair.right?.text === "also theirs")).toBe(true);
  });

  it("says why when there are no blocks to show", () => {
    expect(conflictView("no markers here\n")).toBeNull();
    expect(conflictSentence({ stages: "13", text: "" })).toBe("Conflict: deleted on your side, changed on the incoming side.");
    expect(conflictSentence({ stages: "12", text: "" })).toBe("Conflict: changed on your side, deleted on the incoming side.");
    expect(conflictSentence({ stages: "123", text: "", binary: true })).toBe("Conflict: binary file, no text diff.");
    expect(conflictSentence({ stages: "123", text: "resolved\n" })).toBe("No conflict markers left. Stage the file to mark it resolved.");
    expect(conflictSentence({ stages: "123", text: file })).toBeNull();
  });

  it("reads a marker that never closes as plain text", () => {
    expect(conflictView("<<<<<<< HEAD\nours\n=======\n")).toBeNull();
  });
});
