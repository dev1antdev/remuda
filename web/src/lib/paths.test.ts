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

const paths = (text: string) => findPaths(text).map((m) => m.path);

describe("findPaths", () => {
  it("keeps a :line suffix and strips trailing punctuation", () => {
    const [m] = findPaths("edit internal/server/chat.go:42 now");
    expect(m).toMatchObject({ text: "internal/server/chat.go:42", path: "internal/server/chat.go", line: 42 });
    expect(findPaths("(see docs/README.md).")[0]).toMatchObject({ path: "docs/README.md", line: undefined });
  });
  it("finds absolute, home and bare names with a known extension, in any alphabet", () => {
    expect(paths("~/.claude/settings.json")).toEqual(["~/.claude/settings.json"]);
    expect(paths("/var/log/remuda.log")).toEqual(["/var/log/remuda.log"]);
    expect(paths("run main.go")).toEqual(["main.go"]);
    expect(paths("Обновил README.md и docs/заметки.md, см. Заметки.md")).toEqual(["README.md", "docs/заметки.md", "Заметки.md"]);
  });
  it("links only names a file would have: a folder or a slashed phrase is none", () => {
    expect(paths("look in internal/server and web/src/features/files/")).toEqual([]);
    expect(paths("Approve/Reject/Mark, Send/Send, 1/1, v7/v7/v7, и/или, /dev/null, /usr/local/bin/remuda")).toEqual([]);
    expect(paths("it is docs/. now")).toEqual([]);
    expect(paths("~/.claude/projects holds them")).toEqual([]);
  });
  it("finds dotfiles, the dot kept", () => {
    expect(paths("copy .env.example to .env and check .gitignore.")).toEqual([".env.example", ".env", ".gitignore"]);
    expect(paths("settings in .mcp.json and ~/.zshrc")).toEqual([".mcp.json", "~/.zshrc"]);
    expect(paths("config/.env.local too")).toEqual(["config/.env.local"]);
    expect(paths("the .md files and .env2")).toEqual([]);
  });
  it("never takes the path part of a URL, nor a version number", () => {
    expect(findPaths("see https://x.com/a/b.go")).toEqual([]);
    expect(findPaths("version 1.2.3 is out")).toEqual([]);
  });
});
