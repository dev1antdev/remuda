import { describe, expect, it } from "vitest";
import type { ReviewFile } from "./api";
import { bulkOf, commitAllPaths, conflictWords, discardAsk, discardKind, pathOfKey, rowActions, rowFor, rowsOf, shownOrder, wroteWords } from "./groups";

const file = (path: string, more: Partial<ReviewFile> = {}): ReviewFile => ({ path, added: 1, removed: 0, fresh: true, ...more });

describe("the groups", () => {
  const files = [
    file("both.go", { index: "M", work: "M", staged: { added: 2, removed: 1 }, unstaged: { added: 1, removed: 0 }, added: 3, removed: 1 }),
    file("staged.go", { index: "A", staged: { added: 4, removed: 0 } }),
    file("edited.go", { work: "M", unstaged: { added: 1, removed: 1 } }),
    file("gone.txt", { index: "D", staged: { added: 0, removed: 3 } }),
    file("new.ts", { untracked: true }),
    file("clash.go", { conflict: "UU" }),
  ];
  const groups = rowsOf(files);

  it("puts a file partly staged in both groups, each side with its own counts and diff", () => {
    expect(groups.staged.map((row) => row.file.path)).toEqual(["both.go", "staged.go", "gone.txt"]);
    expect(groups.unstaged.map((row) => row.file.path)).toEqual(["both.go", "edited.go"]);
    const [stagedSide] = groups.staged;
    const [unstagedSide] = groups.unstaged;
    expect([stagedSide!.stat, stagedSide!.from, stagedSide!.letter]).toEqual([{ added: 2, removed: 1 }, { side: "staged" }, "M"]);
    expect([unstagedSide!.stat, unstagedSide!.from]).toEqual([{ added: 1, removed: 0 }, { side: "unstaged" }]);
    expect(groups.staged[1]!.from).toEqual({}); // a file wholly staged keeps the lens's diff since the last look
    expect(stagedSide!.key).not.toBe(unstagedSide!.key);
    expect(pathOfKey(stagedSide!.key)).toBe("both.go");
  });

  it("keeps conflicts and new files apart, in better-changes' order", () => {
    expect(groups.conflict.map((row) => row.file.path)).toEqual(["clash.go"]);
    expect(groups.untracked.map((row) => row.file.path)).toEqual(["new.ts"]);
    expect(shownOrder(groups).map((row) => row.group)).toEqual(["conflict", "staged", "staged", "staged", "unstaged", "unstaged", "untracked"]);
  });

  it("opens a path on its working tree's side when nothing says which", () => {
    expect(rowFor(groups, "both.go")?.group).toBe("unstaged");
    expect(rowFor(groups, "staged.go")?.group).toBe("staged");
    expect(rowFor(groups, "missing")).toBeUndefined();
  });

  it("offers what each row can do, a conflict only its resolution", () => {
    expect(rowActions(groups.conflict[0]!)).toEqual(["stage"]);
    expect(rowActions(groups.staged[0]!)).toEqual(["unstage", "discard"]);
    expect(rowActions(groups.untracked[0]!)).toEqual(["stage", "discard"]);
    expect([bulkOf("conflict"), bulkOf("staged"), bulkOf("unstaged"), bulkOf("untracked")]).toEqual([null, "unstage", "stage", "stage"]);
  });

  it("says what a discard takes, naming the file", () => {
    expect(discardKind(groups.unstaged[0]!)).toBe("unstaged");
    expect(discardKind(groups.staged[0]!)).toBe("all");
    expect(discardKind(groups.untracked[0]!)).toBe("untracked");
    expect(discardAsk(groups.unstaged[0]!).text).toContain("What is staged stays");
    expect(discardAsk(groups.unstaged[1]!).text).toContain("goes back to how the last commit has it");
    expect(discardAsk(groups.staged[1]!).text).toMatch(/^Delete staged\.go\? It is new/);
    expect(discardAsk(groups.untracked[0]!)).toEqual({ text: "Delete new.ts? It was never committed, so it cannot be brought back.", button: "Delete the file", done: "Deleted new.ts." });
  });

  it("stages for Commit all what is listed and not staged, never a conflict", () => {
    expect(commitAllPaths(groups)).toEqual(["both.go", "edited.go", "new.ts"]);
  });

  it("says a conflict's kind and what a write did", () => {
    expect([conflictWords("UU"), conflictWords("DU"), conflictWords("??")]).toEqual(["both edited", "deleted by you", "in conflict"]);
    expect(wroteWords("stage", { paths: ["a.go"] })).toBe("Staged a.go.");
    expect(wroteWords("unstage", { paths: ["a.go", "b.go"] })).toBe("Unstaged 2 files.");
    expect(wroteWords("stage", { paths: ["a.go", "b.go"], skipped: ["c.go"] })).toBe("Staged 2 files. c.go is in conflict and was left as it is.");
  });

  it("reads a summary from before git's status as unstaged edits", () => {
    expect(rowsOf([file("old.go")]).unstaged[0]!.stat).toEqual({ added: 1, removed: 0 });
  });
});
