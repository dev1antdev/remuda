import { describe, expect, it } from "vitest";
import { closeAsk, leftoverWords, madeWorktree, removable, removedWords, sizeWords, tidyWords, type Leftover } from "./tidy";

const left = (more: Partial<Leftover> = {}): Leftover => ({ path: "/h/.herdr/worktrees/app/task-1", repo: "app", name: "task-1", branch: "task-1", kb: 2048, ...more });

describe("madeWorktree", () => {
  it("is a linked worktree under ~/.herdr/worktrees/<repository>/<name>, nothing else", () => {
    expect(madeWorktree({ linked: true, checkout_path: "/Users/me/.herdr/worktrees/app/task-1" })).toBe(true);
    expect(madeWorktree({ linked: false, checkout_path: "/Users/me/.herdr/worktrees/app/task-1" })).toBe(false);
    expect(madeWorktree({ linked: true, checkout_path: "/Users/me/code/app-wt" })).toBe(false);
    expect(madeWorktree({ linked: true, checkout_path: "/Users/me/.herdr/worktrees/app" })).toBe(false);
    expect(madeWorktree(undefined)).toBe(false);
  });
});

describe("closeAsk and tidyWords", () => {
  it("says the worktree goes only for one a close takes down", () => {
    expect(closeAsk("Close “x”?", false)).toBe("Close “x”?");
    expect(closeAsk("Close “x”?", true)).toMatch(/^Close “x”\? Its worktree folder is removed too/);
  });
  it("says what became of the folder and of the branch", () => {
    expect(tidyWords({ path: "/w/task-1", branch: "task-1", removed: true, branch_deleted: true })).toEqual({ text: "The worktree task-1 is removed, its branch task-1 too.", error: false });
    expect(tidyWords({ path: "/w/task-1", branch: "task-1", removed: true, branch_deleted: false, branch_kept: "error: not fully merged" }).text).toBe("The worktree task-1 is removed; its branch task-1 stays, not merged yet.");
    expect(tidyWords({ path: "/w/detached", removed: true, branch_deleted: false }).text).toBe("The worktree detached is removed.");
    expect(tidyWords({ path: "/w/task-1", removed: false, branch_deleted: false, kept: "uncommitted changes (M a.txt)" })).toEqual({ text: "The worktree task-1 stays: uncommitted changes (M a.txt).", error: true });
  });
});

describe("the leftovers dialog's words", () => {
  it("lets only a clean worktree git knows go", () => {
    expect(removable(left())).toBe(true);
    expect(removable(left({ dirty: "?? new.txt" }))).toBe(false);
    expect(removable(left({ broken: "no linked worktree here any more", branch: undefined }))).toBe(false);
  });
  it("says a row's branch and size, or why it stays", () => {
    expect(leftoverWords(left())).toBe("task-1 · 2.0 MB");
    expect(leftoverWords(left({ dirty: "M a.txt" }))).toBe("uncommitted: M a.txt");
    expect(leftoverWords(left({ broken: "git cannot read it" }))).toBe("git cannot read it");
    expect(sizeWords(512)).toBe("512 KB");
    expect(sizeWords(300 * 1024)).toBe("300 MB");
    expect(sizeWords(3 * 1024 * 1024)).toBe("3.0 GB");
  });
  it("sums up a removal with the first reason a worktree stayed", () => {
    expect(removedWords([{ path: "/w/a", removed: true, branch_deleted: true }, { path: "/w/b", removed: true, branch_deleted: false }])).toEqual({ text: "Removed 2 worktrees.", error: false });
    expect(removedWords([{ path: "/w/a", removed: true, branch_deleted: true }, { path: "/w/b", removed: false, branch_deleted: false, kept: "not a leftover worktree now" }])).toEqual({ text: "Removed 1 worktree; 1 stayed — b: not a leftover worktree now.", error: true });
  });
});
