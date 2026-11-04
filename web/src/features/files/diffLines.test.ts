import { describe, expect, it } from "vitest";
import { changeCounts, diffLines } from "./diffLines";

const diff = [
  "diff --git a/q.sql b/q.sql",
  "index 1111111..2222222 100644",
  "--- a/q.sql",
  "+++ b/q.sql",
  "@@ -1,3 +1,3 @@",
  " select 1;",
  "--- an old note",
  "+-- a new note",
  " select 'a",
  "\\ No newline at end of file",
  "",
].join("\n");

const kinds = (text: string, lang = "") => diffLines(text, lang).map((line) => line.kind);
const words = (line: { tokens?: { kind: string; text: string }[] }) => (line.tokens ?? []).filter((t) => t.kind !== "plain").map((t) => `${t.kind}:${t.text}`);

describe("diffLines", () => {
  it("reads the header, the hunk and its lines; a removed line starting with -- is a removed line", () => {
    expect(kinds(diff)).toEqual(["meta", "meta", "meta", "meta", "hunk", "context", "del", "add", "context", "note"]);
    expect(changeCounts(diffLines(diff))).toEqual({ add: 1, del: 1 });
  });
  it("reads a second file's header as a header again", () => {
    const two = `${diff}diff --git a/b.sql b/b.sql\n--- a/b.sql\n+++ b/b.sql\n@@ -1 +1 @@\n-x\n+y\n`;
    expect(kinds(two).slice(10)).toEqual(["meta", "meta", "meta", "hunk", "del", "add"]);
    expect(changeCounts(diffLines(two))).toEqual({ add: 2, del: 2 });
  });
  it("colours each side of a hunk as one text, the marker left out, and leaves the header and the note plain", () => {
    const go = ["--- a/x.go", "+++ b/x.go", "@@ -1,3 +1,3 @@", " /* a comment", "-   that ends */ return 1", "+   that ends */ return 2", " }"].join("\n");
    const lines = diffLines(go, "go");
    expect(lines[0]!.tokens).toBeUndefined();
    expect(words(lines[3]!)).toEqual(["comment:/* a comment"]);
    expect(words(lines[4]!)).toEqual(["comment:   that ends */", "keyword:return", "number:1"]);
    expect(words(lines[5]!)).toEqual(["comment:   that ends */", "keyword:return", "number:2"]);
    expect(diffLines(diff, "sql")[9]!.tokens).toBeUndefined();
  });
  it("colours nothing without a language", () => {
    expect(diffLines(diff).some((line) => line.tokens)).toBe(false);
  });
});
