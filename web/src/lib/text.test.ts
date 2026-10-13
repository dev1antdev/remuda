import { describe, expect, it } from "vitest";
import { basename, dirname, errMsg, plural, shellQuote } from "./text";

describe("text", () => {
  it("basename is the last segment, a trailing slash ignored", () => {
    expect(basename("/a/b/c.ts")).toBe("c.ts");
    expect(basename("/a/b/")).toBe("b");
    expect(basename("plain")).toBe("plain");
  });
  it("dirname keeps the slash and is empty for a bare name", () => {
    expect(dirname("/a/b/c.ts")).toBe("/a/b/");
    expect(dirname("c.ts")).toBe("");
  });
  it("plural counts", () => {
    expect(plural(1, "file")).toBe("1 file");
    expect(plural(3, "file")).toBe("3 files");
    expect(plural(0, "commit")).toBe("0 commits");
  });
  it("shellQuote leaves a plain path alone and quotes the rest", () => {
    expect(shellQuote("/a/b.txt")).toBe("/a/b.txt");
    expect(shellQuote("/a/my file's.txt")).toBe("'/a/my file'\\''s.txt'");
  });
  it("errMsg takes an Error's message, else the value as text", () => {
    expect(errMsg(new Error("boom"))).toBe("boom");
    expect(errMsg("plain")).toBe("plain");
    expect(errMsg(404)).toBe("404");
  });
});
