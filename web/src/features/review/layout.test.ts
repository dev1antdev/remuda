import { describe, expect, it } from "vitest";
import type { ReviewFile } from "./api";
import { rowsOf } from "./groups";
import { layoutGroups, visibleKeys } from "./layout";

const file = (path: string, more: Partial<ReviewFile> = {}): ReviewFile => ({ path, added: 1, removed: 0, fresh: true, ...more });
const groups = rowsOf([
  file("src/app/a.go", { work: "M", unstaged: { added: 1, removed: 0 } }),
  file("src/app/b.go", { work: "M", unstaged: { added: 1, removed: 0 }, fresh: false }),
  file("README.md", { index: "M", staged: { added: 1, removed: 0 } }),
  file("new.ts", { untracked: true }),
]);

describe("the groups' layout", () => {
  it("leaves out an empty group and keeps the order", () => {
    const layouts = layoutGroups(groups, { tree: false, closed: new Set(), looked: false, unfolded: new Set() });
    expect(layouts.map((layout) => layout.group)).toEqual(["staged", "unstaged", "untracked"]);
    expect(visibleKeys(layouts)).toEqual(["staged:README.md", "unstaged:src/app/a.go", "unstaged:src/app/b.go", "untracked:new.ts"]);
  });

  it("folds what did not change since the look under its group until asked", () => {
    const folded = layoutGroups(groups, { tree: false, closed: new Set(), looked: true, unfolded: new Set() });
    const unstaged = folded.find((layout) => layout.group === "unstaged")!;
    expect([unstaged.total, unstaged.unchanged, unstaged.items.length]).toEqual([2, 1, 1]);
    expect(visibleKeys(folded)).not.toContain("unstaged:src/app/b.go");
    expect(visibleKeys(layoutGroups(groups, { tree: false, closed: new Set(), looked: true, unfolded: new Set(["unstaged"]) }))).toContain("unstaged:src/app/b.go");
  });

  it("draws a tree per group, a closed folder's files left out of the walk", () => {
    const open = layoutGroups(groups, { tree: true, closed: new Set(), looked: false, unfolded: new Set() });
    const unstaged = open.find((layout) => layout.group === "unstaged")!;
    expect(unstaged.items.map((item) => (item.kind === "folder" ? item.name : item.name))).toEqual(["src/app/", "a.go", "b.go"]);
    const closed = layoutGroups(groups, { tree: true, closed: new Set(["unstaged:src/app"]), looked: false, unfolded: new Set() });
    expect(visibleKeys(closed)).toEqual(["staged:README.md", "untracked:new.ts"]);
  });
});
