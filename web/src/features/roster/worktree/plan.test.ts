import { describe, expect, it } from "vitest";
import { badBranch, createWords, fill, folderFor, fromPR, pickOf, planOf, slug, titleSlug, type Pick, type Where } from "./plan";
import { baseChoices, emptyWords, items, jumpFor, parseRef, prFor, repositoriesOf, tilde, type Lists, type Source } from "./sources";
import type { Machine, Session } from "../api";

const src: Source = {
  root: "/Users/me/code/app",
  name: "app",
  origin: "https://github.com/acme/storefront.git",
  github: "acme/storefront",
  base: "origin/main",
  local: [{ name: "main", at: 3 }, { name: "fix", at: 2 }, { name: "local-one", at: 1 }],
  remote: [{ name: "main", at: 3 }, { name: "only-on-origin", at: 1 }],
  checkouts: [{ path: "/Users/me/code/app", branch: "main", main: true }, { path: "/Users/me/.herdr/worktrees/app/fix", branch: "fix" }],
};
const where: Where = { home: "/Users/me", root: "/Users/me/.herdr/worktrees", location: "" };
const lists: Lists = {
  issues: [{ number: 198, title: "Saved carts for signed-in shoppers" }, { number: 203, title: "Checkout redesign" }],
  prs: [{ number: 212, title: "Add saved carts", head: "dev/feat/198", closes: [198] }, { number: 207, title: "Redesign", head: "checkout", cross_repo: true }],
};
const issueBranch = (number: number) => `dev/feat/${number}`;

describe("the rows", () => {
  it("lists the branches checked out, local and only on origin, filtered", () => {
    const rows = items("branch", "", src, lists, "/Users/me", (i) => issueBranch(i.number));
    expect(rows.map((r) => [r.tag, r.label, r.sub])).toEqual([
      ["Open", "fix", "~/.herdr/worktrees/app/fix"],
      ["Local", "local-one", "Local branch"],
      ["Remote", "only-on-origin", "origin/only-on-origin · not checked out here"],
    ]);
    expect(items("branch", "ORIGIN", src, lists, "/Users/me", () => "").map((r) => r.label)).toEqual(["only-on-origin"]);
    expect(items("new", "", src, lists, "/Users/me", () => "")).toEqual([]);
  });
  it("says which pull request works on an issue", () => {
    const rows = items("issue", "", src, lists, "/Users/me", (i) => issueBranch(i.number));
    expect(rows.map((r) => r.sub)).toEqual(["#198 · PR #212 works on it", "#203"]);
    expect(prFor(lists.prs, 203, "checkout")?.number).toBe(207);
  });
  it("offers a number neither list holds to be looked up", () => {
    const [row] = items("pr", "#999", src, lists, "/Users/me", () => "");
    expect(row).toMatchObject({ kind: "lookup", want: "pr", number: 999, tag: "GitHub" });
    expect(items("pr", "https://github.com/acme/storefront/pull/212", src, lists, "/Users/me", () => "")[0]).toMatchObject({ kind: "pr", number: 212 });
  });
  it("jumps to the tab a pasted reference belongs on", () => {
    expect(parseRef("https://github.com/acme/storefront/issues/198")).toEqual({ number: 198, kind: "issue" });
    expect(parseRef("  #42 ")).toEqual({ number: 42 });
    expect(parseRef("feature-1")).toBeNull();
    expect(jumpFor("branch", "#212", lists)).toBe("pr");
    expect(jumpFor("new", "#198", lists)).toBe("issue");
    expect(jumpFor("branch", "198", lists)).toBeNull();
    expect(jumpFor("issue", "#212", lists)).toBeNull();
    expect(jumpFor("branch", "https://github.com/acme/storefront/pull/5", null)).toBe("pr");
  });
  it("says why a list is empty", () => {
    expect(emptyWords("branch", null, true, "", null)).toBe("Reading branches…");
    expect(emptyWords("issue", { ...src, github: "" }, false, "", null)).toBe("This repository's origin isn't on GitHub.");
    expect(emptyWords("pr", src, false, "", null)).toBe("Loading from GitHub…");
    expect(emptyWords("pr", src, false, "", { issues: [], prs: [], trouble: "gh is not installed on GOG" })).toBe("gh is not installed on GOG");
    expect(emptyWords("branch", src, false, "", lists)).toBe("Nothing matches.");
  });
  it("offers the default base first", () => {
    expect(baseChoices(src)).toEqual(["origin/main", "origin/only-on-origin", "main", "fix", "local-one"]);
  });
});

describe("the plan", () => {
  const plan = (branch: string, pick: Pick, base = "") => planOf(src, branch, base, pick, where);
  it("says what Create will do", () => {
    expect(plan("fix", { kind: "branch", branch: "fix" }).note).toBe("Already checked out at ~/.herdr/worktrees/app/fix · opens it");
    expect(plan("local-one", { kind: "branch", branch: "local-one" }).note).toBe("Local branch · checks it out");
    expect(plan("only-on-origin", { kind: "branch", branch: "only-on-origin" }).note).toBe("Only on origin · makes a local branch tracking origin/only-on-origin");
    expect(plan("dev/x", { kind: "new" }).note).toBe("New branch from origin/main · ~/.herdr/worktrees/app/dev-x");
    expect(plan("dev/x", { kind: "new" }, "main")).toMatchObject({ action: "new", from: "main", fromBase: true });
    expect(plan("cart", { kind: "pr", number: 212, head: "cart" }).note).toBe("Fetches cart from origin, then tracks it");
    expect(plan("cart", { kind: "pr", number: 207, head: "cart", cross_repo: true }).note).toBe("From a fork · fetches pull/207/head as cart");
    expect(plan("cart-2", { kind: "pr", number: 212, head: "cart" })).toMatchObject({ action: "new", from: "origin/cart", fromBase: false });
    expect(plan("local-one", { kind: "issue", number: 5 }).note).toBe("local-one already exists · checks it out");
  });
  it("says what is wrong with the branch, in amber", () => {
    expect(plan("", { kind: "new" })).toEqual({ action: "error", note: "Name the branch." });
    for (const bad of ["has space", "-x", "a..b", "x.lock", "end/", "a:b"]) expect(plan(bad, { kind: "new" }).action).toBe("error");
    expect(badBranch("dev/feat/198")).toBe(false);
  });
  it("puts a new worktree in the project's own location when it has one", () => {
    expect(folderFor(src, { ...where, location: "/Users/me/trees/" }, "dev/feat/198")).toBe("/Users/me/trees/dev-feat-198");
    expect(slug("Weird.Name_x+y")).toBe("weird-name-x-y");
  });
  it("fills the Details from a row", () => {
    const [issue] = items("issue", "198", src, lists, "/Users/me", (i) => issueBranch(i.number));
    expect(pickOf(issue!, issueBranch)).toEqual({ pick: { kind: "issue", number: 198, pr: 212, head: "dev/feat/198", cross_repo: undefined }, branch: "dev/feat/198", name: "Saved carts for signed-in shoppers" });
    const [other] = items("issue", "203", src, lists, "/Users/me", (i) => issueBranch(i.number));
    expect(pickOf(other!, issueBranch).branch).toBe("dev/feat/203");
    expect(fromPR({ kind: "issue", number: 198, pr: 212, head: "dev/feat/198" }, "dev/feat/198")).toBe("From PR #212 · ");
  });
  it("names the button by the plan", () => {
    expect(createWords(plan("fix", { kind: "branch", branch: "fix" }), false)).toBe("Open worktree");
    expect(createWords(plan("dev/x", { kind: "new" }), false)).toBe("Create worktree");
    expect(createWords(null, true)).toBe("Working…");
  });
});

describe("the templates and the places", () => {
  it("fills a prompt and a branch pattern", () => {
    expect(fill("Work on #{number} in {repo}: “{title}” on {branch}", 198, "Saved carts", "acme/storefront", "dev/feat/198")).toBe("Work on #198 in acme/storefront: “Saved carts” on dev/feat/198");
    expect(fill("dev/{number}-{slug}", 7, "Fix: the cart’s total (again)!", "", "")).toBe("dev/7-fix-the-cart-s-total-again");
    expect(titleSlug("x".repeat(60)).length).toBe(40);
  });
  it("says a path under the home with ~", () => {
    expect(tilde("/Users/me/code/app", "/Users/me/")).toBe("~/code/app");
    expect(tilde("C:\\Users\\me\\code", "C:\\Users\\me")).toBe("~/code");
    expect(tilde("/Users/meow/x", "/Users/me")).toBe("/Users/meow/x");
  });
  it("lists a machine's repositories, the one asked for first", () => {
    const session = (id: string, dir: string, folder: string, linked = false): Session =>
      ({ id, name: id, status: "idle", active_tab_id: "", dir, folder, branch: "main", worktree: { repo_key: folder, repo_name: folder.split("/").pop()!, linked, checkout_path: dir }, tabs: [] }) as Session;
    const machine = {
      id: "local",
      sessions: [session("w2", "/c/web/.wt/x", "/c/web", true), session("w1", "/c/api", "/c/api"), session("w3", "/c/web", "/c/web"), { ...session("w4", "/tmp", "/tmp"), branch: undefined, worktree: undefined }],
    } as unknown as Machine;
    expect(repositoriesOf(machine, "/c/web")).toEqual([
      { root: "/c/web", name: "web", workspace: "w3", repoName: "web" },
      { root: "/c/api", name: "api", workspace: "w1", repoName: "api" },
    ]);
  });
});
