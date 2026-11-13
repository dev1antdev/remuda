import { describe, expect, it } from "vitest";
import { menuFor, type Context, type Row } from "./menus";
import { asksFirst, dangerous, doneWords, initialValues, loseWords, problemOf, requestOf, specFor } from "./ops";

const HEAD = "a".repeat(40);
const OTHER = "b".repeat(40);
const ctx: Context = { branch: "main", head: HEAD, branches: ["main", "fix"], dirty: 0 };
const labels = (rows: Row[]) => rows.filter((r) => !("sep" in r)).map((r) => ("label" in r ? r.label : ""));

describe("menuFor", () => {
  it("offers the checked-out branch no checkout, delete, merge or rebase of itself", () => {
    expect(labels(menuFor({ kind: "branch", name: "main", hash: HEAD }, ctx))).toEqual(["Rename branch…", "Copy branch name"]);
    expect(labels(menuFor({ kind: "branch", name: "fix", hash: OTHER }, ctx))).toEqual(["Checkout branch", "Rename branch…", "Delete branch…", "Merge into main…", "Rebase main on this branch…", "Copy branch name"]);
  });

  it("checks a remote branch out as its local one when there is one, else asks the local name", () => {
    expect(labels(menuFor({ kind: "remote", name: "origin/fix", hash: OTHER }, ctx))[0]).toBe("Checkout fix");
    expect(menuFor({ kind: "remote", name: "origin/new", hash: OTHER }, ctx)[0]).toMatchObject({ label: "Checkout branch…", action: { type: "checkout-remote", branch: "new" } });
    expect(labels(menuFor({ kind: "remote", name: "origin/main", hash: HEAD }, ctx))).not.toContain("Checkout main");
  });

  it("offers a commit what git-graph does, less what HEAD already is, and nothing that reaches a remote", () => {
    const here = labels(menuFor({ kind: "commit", hash: HEAD, subject: "s", parents: [OTHER] }, ctx));
    expect(here).toEqual(["Add tag…", "Create branch…", "Revert…", "Reset main to this commit…", "Copy commit hash", "Copy commit subject"]);
    const there = labels(menuFor({ kind: "commit", hash: OTHER, subject: "s", parents: [HEAD, HEAD] }, { ...ctx, branch: "" }));
    expect(there).toContain("Cherry pick…");
    expect(there).toContain("Merge into HEAD…");
    const every = [...here, ...there, ...labels(menuFor({ kind: "tag", name: "v1", hash: HEAD }, ctx)), ...labels(menuFor({ kind: "uncommitted" }, ctx))];
    expect(every.some((l) => /\b(push|pull|fetch|stash)\b|^commit…$/i.test(l))).toBe(false);
  });
});

describe("the dialogs and requests", () => {
  it("runs a local checkout at once only with nothing uncommitted", () => {
    expect(asksFirst({ type: "checkout", name: "fix" }, ctx)).toBe(false);
    expect(asksFirst({ type: "checkout", name: "fix" }, { ...ctx, dirty: 2 })).toBe(true);
    expect(asksFirst({ type: "tag-add", hash: HEAD }, ctx)).toBe(true);
  });

  it("names the files a hard reset loses and turns its button red only then", () => {
    const spec = specFor({ type: "reset", hash: OTHER }, ctx, [{ status: " M", path: "a.go" }, { status: "M ", path: "b.go" }]);
    const values = initialValues(spec);
    expect(values.mode).toBe("mixed");
    expect(dangerous(spec, values)).toBe(false);
    expect(dangerous(spec, { ...values, mode: "hard" })).toBe(true);
    expect(loseWords(spec.lose!)).toBe("It discards the uncommitted changes to 2 files: a.go, b.go.");
    expect(loseWords(Array.from({ length: 10 }, (_, i) => `f${i}`))).toMatch(/10 files: f0, .*f7 and 2 more\.$/);
    expect(loseWords([])).toBe("Nothing uncommitted is lost.");
  });

  it("asks a second time, in red, before deleting a branch git says is not merged", () => {
    const first = specFor({ type: "branch-delete", name: "fix" }, ctx);
    const forced = specFor({ type: "branch-delete", name: "fix" }, ctx, [], true);
    expect(first.danger && forced.danger).toBe(true);
    expect(forced.detail).toMatch(/not merged into main/);
    expect(requestOf({ type: "branch-delete", name: "fix" }, {}, ctx).force).toBe(false);
    expect(requestOf({ type: "branch-delete", name: "fix" }, {}, ctx, true).force).toBe(true);
  });

  it("refuses a name git would read as an option and sends what the graph showed", () => {
    const spec = specFor({ type: "branch-create", hash: OTHER }, ctx);
    expect(problemOf(spec, { name: "" })).toBe("Branch name is empty.");
    expect(problemOf(spec, { name: "-D" })).toMatch(/cannot start/);
    expect(problemOf(spec, { name: "a b" })).toMatch(/space/);
    expect(problemOf(spec, { name: "fix/x" })).toBe("");
    expect(requestOf({ type: "branch-create", hash: OTHER }, { name: " fix/x ", checkout: true }, ctx)).toEqual({ op: "branch-create", sha: OTHER, name: "fix/x", checkout: true, head: HEAD, branch: "main" });
    expect(requestOf({ type: "merge", hash: OTHER, what: "commit" }, { noff: true }, ctx)).toMatchObject({ op: "merge", sha: OTHER, no_ff: true });
    expect(requestOf({ type: "merge", ref: "origin/fix", what: "x" }, {}, ctx)).toMatchObject({ op: "merge", ref: "origin/fix" });
    expect(doneWords({ type: "checkout-remote", name: "origin/new", branch: "new" }, { name: "new" })).toBe("Checked out new, tracking origin/new");
  });
});
