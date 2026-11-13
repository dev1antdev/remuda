import { describe, expect, it } from "vitest";
import type { Commit, GitLog } from "./api";
import { historyOf, rowCommit, withPage, withUncommitted } from "./history";
import { foldLanes } from "./lanes";

const commit = (hash: string, parents: string[]): Commit => ({ hash, parents, author: "Ada", email: "a@x", time: 0, subject: hash });
const ALL = [commit("c", ["b"]), commit("x", ["b"]), commit("b", ["a"]), commit("a", [])];
const log = (commits: Commit[], skip: number, more: boolean, fingerprint = "f1"): GitLog => ({ repo: true, head: "c", branch: "main", fingerprint, dirty: 0, conflicts: 0, commits, more, skip, branches: ["main"], remotes: [] });

describe("the history's pages", () => {
  it("goes on from the fold's state, as one fold of the whole would", () => {
    const first = historyOf(log(ALL.slice(0, 2), 0, true), true);
    const both = withPage(first, log(ALL.slice(2), 2, false))!;
    expect(both.commits.map((c) => c.hash)).toEqual(["c", "x", "b", "a"]);
    expect(both.rows).toEqual(foldLanes([{ hash: "*", parents: ["c"], dashed: true }, ...ALL]).rows);
    expect(both.log.more).toBe(false);
  });

  it("reads again from the top once the refs moved between pages", () => {
    const first = historyOf(log(ALL.slice(0, 2), 0, true), false);
    expect(withPage(first, log(ALL.slice(2), 2, false, "f2"))).toBeNull();
    expect(withPage(first, log(ALL.slice(2), 3, false))).toBeNull();
  });

  it("lays the uncommitted row out only when it comes or goes, and never before a first commit", () => {
    const clean = historyOf(log(ALL, 0, false), false);
    expect(withUncommitted(clean, false)).toBe(clean);
    const dirty = withUncommitted(clean, true);
    expect(dirty.uncommitted).toBe(true);
    expect(dirty.rows).toHaveLength(ALL.length + 1);
    expect(rowCommit(dirty, 0)).toBeNull();
    expect(rowCommit(dirty, 1)?.hash).toBe("c");
    expect(rowCommit(clean, 0)?.hash).toBe("c");
    expect(historyOf({ ...log([], 0, false), head: "" }, true).uncommitted).toBe(false);
  });
});
