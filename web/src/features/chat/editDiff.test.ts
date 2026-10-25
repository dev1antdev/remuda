import { describe, expect, it } from "vitest";
import { editRows, editsOf, foldRows, NO_NEWLINE, type EditRow } from "./editDiff";

const kinds = (rows: EditRow[]) => rows.map((r) => r.kind).join(" ");
const texts = (rows: EditRow[]) => rows.map((r) => `${r.kind === "del" ? "-" : r.kind === "add" ? "+" : " "}${r.text}`);

describe("editsOf", () => {
  it("reads the one edit of an Edit, with replace_all", () => {
    expect(editsOf("Edit", { file_path: "/r/a.go", old_string: "x", new_string: "y" })).toEqual([{ old: "x", new: "y", replaceAll: false }]);
    expect(editsOf("Edit", { file_path: "/r/a.go", old_string: "x", new_string: "y", replace_all: true })[0]?.replaceAll).toBe(true);
  });
  it("reads every edit of a MultiEdit and skips a broken one", () => {
    const edits = editsOf("MultiEdit", { file_path: "/r/a.go", edits: [{ old_string: "a", new_string: "b" }, { old_string: "c", new_string: "d", replace_all: true }, { old_string: "e", new_string: "f" }, { old_string: 1 }] });
    expect(edits).toHaveLength(3);
    expect(edits[1]).toEqual({ old: "c", new: "d", replaceAll: true });
  });
  it("is none for a cut (string) input, another tool, or nothing", () => {
    expect(editsOf("Edit", '{"file_path":"/r/a.go","old_string":"…')).toEqual([]);
    expect(editsOf("Write", { file_path: "/r/a.go", content: "x" })).toEqual([]);
    expect(editsOf("Edit", undefined)).toEqual([]);
    expect(editsOf("Edit", { file_path: "/r/a.go", old_string: "x" })).toEqual([]);
  });
});

describe("editRows", () => {
  it("is all context for identical texts", () => {
    expect(kinds(editRows("a\nb\nc", "a\nb\nc"))).toBe("ctx ctx ctx");
  });
  it("shows a pure insertion as added lines in place", () => {
    expect(texts(editRows("a\nc", "a\nb\nc"))).toEqual([" a", "+b", " c"]);
  });
  it("shows a pure deletion as removed lines in place", () => {
    expect(texts(editRows("a\nb\nc", "a\nc"))).toEqual([" a", "-b", " c"]);
  });
  it("marks the word that changed on both sides of a one-word change", () => {
    const rows = editRows("const limit = 10;", "const limit = 20;");
    expect(kinds(rows)).toBe("del add");
    const [del, add] = rows;
    expect(del!.mark).toEqual({ head: 14, tail: 1 });
    expect("const limit = 10;".slice(del!.mark!.head, "const limit = 10;".length - del!.mark!.tail)).toBe("10");
    expect("const limit = 20;".slice(add!.mark!.head, "const limit = 20;".length - add!.mark!.tail)).toBe("20");
  });
  it("pairs a multi-line replacement run by run, the extra added lines unmarked", () => {
    const rows = editRows("func a() {\n\treturn 1\n}", "func a() int {\n\tx := 1\n\treturn x\n}");
    expect(texts(rows)).toEqual(["-func a() {", "-\treturn 1", "+func a() int {", "+\tx := 1", "+\treturn x", " }"]);
    expect(rows[0]!.mark).not.toBeNull();
    expect(rows[1]!.mark).not.toBeNull();
    expect(rows[4]!.mark).toBeNull();
  });
  it("notes a trailing newline only one side has, as git does", () => {
    expect(texts(editRows("a\nb", "a\nb\n"))).toEqual([" a", "-b", ` ${NO_NEWLINE}`, "+b"]);
    expect(texts(editRows("a\nb\n", "a\nb"))).toEqual([" a", "-b", "+b", ` ${NO_NEWLINE}`]);
    expect(kinds(editRows("a\nb\n", "a\nb\n"))).toBe("ctx ctx");
  });
  it("reads an empty old text as all additions and an empty new text as all removals", () => {
    expect(kinds(editRows("", "a\nb"))).toBe("add add");
    expect(kinds(editRows("a\nb", ""))).toBe("del del");
    expect(editRows("", "")).toEqual([]);
  });
  it("gives up the pairing on two texts too long to table, as a whole replacement", () => {
    const old = Array.from({ length: 700 }, (_, i) => `line ${i}`).join("\n");
    const fresh = old.replace("line 350", "line 350 changed");
    const rows = editRows(old, fresh);
    expect(rows.filter((r) => r.kind === "del")).toHaveLength(700);
    expect(rows.filter((r) => r.kind === "add")).toHaveLength(700);
  });
});

describe("foldRows", () => {
  it("keeps every row under the cap and counts the rest past it", () => {
    const rows = editRows("a\nb\nc", "a\nx\nc");
    expect(foldRows(rows, 24)).toEqual({ shown: rows, more: 0 });
    expect(foldRows(rows, 2)).toEqual({ shown: rows.slice(0, 2), more: 2 });
  });
});
