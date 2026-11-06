import { describe, expect, it } from "vitest";
import { includedWords } from "./included";

describe("includedWords", () => {
  it("says nothing for a repository without the file", () => {
    expect(includedWords(undefined)).toBe("");
    expect(includedWords({ listed: false })).toBe("");
  });

  it("names the files copied, the first three and how many more", () => {
    expect(includedWords({ listed: true, copied: [".env"] })).toBe("Copied 1 file from .worktreeinclude: .env");
    expect(includedWords({ listed: true, copied: [".env", ".npmrc", "config/local.json", "certs/dev.pem", "x"] })).toBe("Copied 5 files from .worktreeinclude: .env, .npmrc, config/local.json +2");
  });

  it("says when the patterns matched nothing, too much, or only files it left out", () => {
    expect(includedWords({ listed: true })).toBe(".worktreeinclude matches no ignored file: nothing to copy");
    expect(includedWords({ listed: true, too_many: 5400 })).toBe(".worktreeinclude matches 5400 ignored files, more than a worktree gets: nothing was copied — narrow its patterns");
    expect(includedWords({ listed: true, skipped: [{ path: ".env", why: "already in the worktree" }] })).toBe("Nothing copied from .worktreeinclude: 1 file is already in the worktree or leads out of it");
  });

  it("puts a failure first", () => {
    expect(includedWords({ listed: true, copied: [".env"] }, "the new worktree is not there")).toBe("The worktree is made, but the files .worktreeinclude names were not copied: the new worktree is not there");
  });
});
