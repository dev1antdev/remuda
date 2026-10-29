import { describe, expect, it } from "vitest";
import { applyPick, commandItems, MAX_SHOWN, modelCommand, pathItems, tokenAt, type Command } from "./complete";

describe("tokenAt", () => {
  it("sees a slash command only at the very start, up to the first space", () => {
    expect(tokenAt("/", 1)).toEqual({ kind: "command", start: 0, end: 1, query: "" });
    expect(tokenAt("/comp", 5)).toEqual({ kind: "command", start: 0, end: 5, query: "comp" });
    expect(tokenAt("/compact focus on the tests", 27)).toBeNull();
    expect(tokenAt("look at /etc", 12)).toBeNull();
    expect(tokenAt("\n/x", 3)).toBeNull();
    expect(tokenAt("/comp", 2)).toEqual({ kind: "command", start: 0, end: 2, query: "c" });
  });

  it("sees an @ after the start or a space, never inside a word", () => {
    expect(tokenAt("@", 1)).toEqual({ kind: "path", start: 0, end: 1, query: "" });
    expect(tokenAt("see @web/src", 12)).toEqual({ kind: "path", start: 4, end: 12, query: "web/src" });
    expect(tokenAt("mail me@example.com", 19)).toBeNull();
    expect(tokenAt("see @a.ts and more", 10)).toBeNull();
    expect(tokenAt("see @a.ts and more", 9)).toEqual({ kind: "path", start: 4, end: 9, query: "a.ts" });
    expect(tokenAt("see @a.ts and more", 8)).toEqual({ kind: "path", start: 4, end: 8, query: "a.t" });
  });

  it("stays inside the draft whatever the caret says", () => {
    expect(tokenAt("/x", 99)).toEqual({ kind: "command", start: 0, end: 2, query: "x" });
    expect(tokenAt("", 0)).toBeNull();
  });
});

describe("applyPick", () => {
  it("replaces the token, adds a space and lands the caret after it", () => {
    expect(applyPick("/comp", { kind: "command", start: 0, end: 5, query: "comp" }, "/compact")).toEqual({ draft: "/compact ", caret: 9 });
    expect(applyPick("see @web and more", { kind: "path", start: 4, end: 8, query: "web" }, "@web/src/x.ts")).toEqual({ draft: "see @web/src/x.ts and more", caret: 18 });
  });
});

const commands: Command[] = [
  { name: "compact", description: "Free up context by summarizing the conversation so far", source: "builtin" },
  { name: "model", description: "Set the AI model for Claude Code", source: "builtin" },
  { name: "fix", description: "Fix the failing test", source: "user" },
  { name: "verify", description: "Run the full check", source: "project" },
];

describe("commandItems", () => {
  it("lists everything for an empty query, in the given order, with the source as a tag", () => {
    const items = commandItems(commands, "");
    expect(items.map((i) => i.title)).toEqual(["/compact", "/model", "/fix", "/verify"]);
    expect(items[0]!.insert).toBe("/compact");
    expect(items[0]!.tag).toBe("");
    expect(items[2]!.tag).toBe("yours");
    expect(items[3]!.tag).toBe("this project");
  });

  it("ranks a name's start first and finds a word of the description", () => {
    expect(commandItems(commands, "mo").map((i) => i.title)).toEqual(["/model"]);
    expect(commandItems(commands, "check").map((i) => i.title)).toEqual(["/verify"]);
    expect(commandItems(commands, "zzz")).toEqual([]);
  });
});

describe("pathItems", () => {
  const paths = ["README.md", "internal/server/chat.go", "web/src/features/chat/Chat.tsx", "web/src/lib/text.ts", ...Array.from({ length: 20 }, (_, i) => `docs/page${i}.md`)];

  it("shows the first rows untyped and ranks the file name above a folder hit", () => {
    expect(pathItems(paths, "")).toHaveLength(MAX_SHOWN);
    expect(pathItems(paths, "")[0]).toEqual({ id: "README.md", title: "README.md", insert: "@README.md" });
    expect(pathItems(paths, "chat").map((i) => i.title)).toEqual(["internal/server/chat.go", "web/src/features/chat/Chat.tsx"]);
    expect(pathItems(paths, "text ts").map((i) => i.title)).toEqual(["web/src/lib/text.ts"]);
    expect(pathItems(paths, "nothing")).toEqual([]);
  });
});

describe("modelCommand", () => {
  it("is Claude Code's own /model with the id", () => {
    expect(modelCommand(" opus ")).toBe("/model opus");
    expect(modelCommand("claude-opus-4-1")).toBe("/model claude-opus-4-1");
  });
});
