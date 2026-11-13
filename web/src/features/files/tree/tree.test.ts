import { describe, expect, it } from "vitest";
import { absoluteOf, childOf, fileMessage, markWords, relativeOf, rowActions, rowTitle, toneOf } from "./tree";

describe("an entry of the tree", () => {
  it("lies under the workspace folder in that machine's own spelling", () => {
    expect(childOf("", "src")).toBe("src");
    expect(childOf("src", "app.go")).toBe("src/app.go");
    expect(absoluteOf("/home/me/app", "src/app.go")).toBe("/home/me/app/src/app.go");
    expect(absoluteOf("/home/me/app/", "src")).toBe("/home/me/app/src");
    expect(absoluteOf("C:\\Users\\me\\app", "src/Form1.cs")).toBe("C:\\Users\\me\\app\\src\\Form1.cs");
    expect(absoluteOf("/home/me/app", "")).toBe("/home/me/app");
    expect(relativeOf("C:\\Users\\me\\app", "src/Form1.cs")).toBe("src\\Form1.cs");
    expect(relativeOf("/home/me/app", "src/app.go")).toBe("src/app.go");
  });

  it("wears its mark's colour, a folder holding changes the changed one", () => {
    expect(toneOf({ name: "a", mark: "?" })).toBe("added");
    expect(toneOf({ name: "a", mark: "A" })).toBe("added");
    expect(toneOf({ name: "a", mark: "M" })).toBe("changed");
    expect(toneOf({ name: "a", mark: "D", gone: true })).toBe("deleted");
    expect(toneOf({ name: "a", mark: "U" })).toBe("conflict");
    expect(toneOf({ name: "src", dir: true, changes: true })).toBe("changed");
    expect(toneOf({ name: "src", dir: true })).toBe("");
    expect(toneOf({ name: "a" })).toBe("");
  });

  it("says in its tooltip what git says of it", () => {
    expect(markWords(undefined)).toBe("");
    expect(rowTitle({ name: "x", mark: "M" }, "src/x")).toBe("src/x\nChanged since HEAD");
    expect(rowTitle({ name: "out", dir: true, ignored: true, link: true }, "out")).toBe("out\nIgnored by git\nA symbolic link");
    expect(rowTitle({ name: "src", dir: true, changes: true }, "src")).toBe("src\nHolds changes");
  });
});

describe("a row's menu", () => {
  const person = { viewOnly: false, editor: true };
  it("opens a file, its editor, its changes, copies and hands it to an agent", () => {
    expect(rowActions({ name: "a.go", mark: "M" }, person)).toEqual(["open", "editor", "changes", "copy-relative", "copy-full", "send"]);
  });
  it("offers no changes for a file git has no diff of, and no Open for a folder", () => {
    expect(rowActions({ name: "a.go" }, person)).toEqual(["open", "editor", "copy-relative", "copy-full", "send"]);
    expect(rowActions({ name: "new.go", mark: "?" }, person)).not.toContain("changes");
    expect(rowActions({ name: "src", dir: true, changes: true }, person)).toEqual(["editor", "copy-relative", "copy-full", "send"]);
  });
  it("has no editor on a machine without one, nor for a file that is gone", () => {
    expect(rowActions({ name: "a.cs" }, { viewOnly: false, editor: false })).not.toContain("editor");
    expect(rowActions({ name: "old.txt", mark: "D", gone: true }, person)).toEqual(["open", "changes", "copy-relative", "copy-full", "send"]);
  });
  it("lets a viewer read and copy alone", () => {
    expect(rowActions({ name: "a.go", mark: "M" }, { viewOnly: true, editor: true })).toEqual(["open", "copy-relative", "copy-full"]);
  });
});

describe("a file handed to an agent", () => {
  it("is the instruction, then the path", () => {
    expect(fileMessage("  Add a test for this  ", "/home/me/app/src/app.go", false)).toBe("Add a test for this\n\nFile: /home/me/app/src/app.go");
    expect(fileMessage("Tidy it", "/home/me/app/src", true)).toBe("Tidy it\n\nFolder: /home/me/app/src");
  });
});
