import { describe, expect, it } from "vitest";
import { findPaths, findUrls } from "./paths";

describe("findUrls", () => {
  it("finds http(s) URLs and the scheme-less dev-server kind, trailing punctuation dropped", () => {
    const urls = findUrls("see https://example.com/a, and localhost:5173/app.");
    expect(urls.map((u) => u.text)).toEqual(["https://example.com/a", "localhost:5173/app"]);
    expect(urls[1]!.url).toBe("http://localhost:5173/app");
  });
  it("needs a port or a path after a bare host", () => {
    expect(findUrls("localhost is fine").length).toBe(0);
  });
});

describe("findPaths", () => {
  it("keeps a :line suffix and strips trailing punctuation", () => {
    const [m] = findPaths("edit internal/server/chat.go:42 now");
    expect(m).toMatchObject({ text: "internal/server/chat.go:42", path: "internal/server/chat.go", line: 42 });
    expect(findPaths("(see docs/README.md).")[0]).toMatchObject({ path: "docs/README.md", line: undefined });
  });
  it("finds absolute, home and bare names with a known extension", () => {
    expect(findPaths("~/.claude/projects")[0]!.path).toBe("~/.claude/projects");
    expect(findPaths("/usr/local/bin/remuda")[0]!.path).toBe("/usr/local/bin/remuda");
    expect(findPaths("run main.go")[0]!.path).toBe("main.go");
  });
  it("never takes the path part of a URL, nor a version number", () => {
    expect(findPaths("see https://x.com/a/b.go")).toEqual([]);
    expect(findPaths("version 1.2.3 is out")).toEqual([]);
  });
});
