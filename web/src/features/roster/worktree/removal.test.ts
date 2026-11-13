import { describe, expect, it } from "vitest";
import { blockedWords, branchGoesByDefault, buttonWords, rows, sharedWords, warnings } from "./removal";
import type { Removal } from "./api";

const clean: Removal = {
  workspace: "w2",
  name: "fix",
  repo_name: "app",
  path: "/Users/me/.herdr/worktrees/app/fix",
  branch: "fix",
  repo: "/Users/me/code/app",
  dirty: 0,
  unpushed: 0,
  unmerged: 0,
  merged: true,
  made: true,
  stamp: "0||0|fix|",
};

describe("the remove dialog", () => {
  it("lays out what goes and what stays", () => {
    expect(rows(clean, true, "/Users/me")).toEqual([
      { what: "folder", label: "Folder", value: "fix", under: "~/.herdr/worktrees/app/", pill: "Deleted", flips: false },
      { what: "branch", label: "Branch", value: "fix", pill: "Deleted", flips: true },
    ]);
    expect(rows({ ...clean, branch: undefined }, false, "/Users/me")).toHaveLength(1);
    expect(branchGoesByDefault(clean)).toBe(true);
    expect(branchGoesByDefault({ ...clean, merged: false })).toBe(false);
    expect(branchGoesByDefault({ ...clean, merged: false, merged_pr: true })).toBe(true);
  });
  it("says in red what would be lost", () => {
    const risky: Removal = { ...clean, dirty: 3, unmerged: 2, merged: false, submodules: ["lib"] };
    expect(warnings(risky, false)).toEqual(["Uncommitted changes in this worktree will be lost.", "Submodule lib has commits that aren't pushed anywhere: they will be lost."]);
    expect(warnings(risky, true)[1]).toBe("2 commits on fix are on no remote and in no other branch: deleting the branch loses them.");
    expect(buttonWords(risky, false, false)).toBe("Remove anyway");
    expect(buttonWords(clean, true, false)).toBe("Remove");
    expect(buttonWords(clean, true, true)).toBe("Removing…");
  });
  it("closes the workspace alone when another one uses the checkout", () => {
    const shared: Removal = { ...clean, dirty: 4, shared: ["api-fix"] };
    expect(rows(shared, true, "/Users/me").map((r) => r.pill)).toEqual(["Kept", "Kept"]);
    expect(warnings(shared, true)).toEqual([]);
    expect(sharedWords(shared)).toBe("api-fix also uses this worktree, so it stays. Only this workspace closes.");
    expect(sharedWords({ ...shared, shared: ["a", "b"] })).toBe("a, b also use this worktree, so it stays. Only this workspace closes.");
    expect(buttonWords(shared, true, false)).toBe("Close workspace");
    expect(blockedWords({ ...shared, working: "Claude Code works or asks there" })).toBe("");
  });
  it("refuses while an agent works there", () => {
    expect(blockedWords({ ...clean, working: "Claude Code works or asks there" })).toBe("Claude Code works or asks there: let it finish, or stop it, before its worktree goes.");
    expect(blockedWords(clean)).toBe("");
  });
});
