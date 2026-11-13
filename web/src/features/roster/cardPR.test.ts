import { describe, expect, it } from "vitest";
import type { PRStatus, PullRequest, Session } from "./api";
import { cardPRs, isLinkClick, moreTip, prHost, prLook, prNumber, prRepo, prTip } from "./cardPR";

const status = (over: Partial<PRStatus> = {}): PRStatus => ({ state: "OPEN", draft: false, mergeable: "MERGEABLE", review: "", checks: { total: 2, pass: 2, fail: 0, pending: 0, skipped: 0 }, at: "2026-11-13T12:00:00Z", ...over });
const pr = (over: Partial<PRStatus> | null = {}, url = "https://github.com/acme/storefront/pull/212"): PullRequest => ({ url, label: "PR #212", ...(over ? { status: status(over) } : {}) });

describe("a pull request's look on the card", () => {
  it("colours each state the way munza's badge does, a merge or a close winning over what was flagged while open", () => {
    expect(prLook(pr({ state: "MERGED", checks: { total: 1, pass: 0, fail: 1, pending: 0, skipped: 0 } }))).toMatchObject({ tone: "violet", icon: "git-merge", label: "Merged" });
    expect(prLook(pr({ state: "CLOSED", draft: true }))).toMatchObject({ tone: "red", icon: "git-pull-request-closed", label: "Closed" });
    expect(prLook(pr({ draft: true }))).toMatchObject({ tone: "dim", icon: "git-pull-request-draft", label: "Draft" });
    expect(prLook(pr({ mergeable: "CONFLICTING" }))).toMatchObject({ tone: "red", label: "Conflicts" });
    expect(prLook(pr({ merge_state: "DIRTY" }))).toMatchObject({ tone: "red", label: "Conflicts" });
    expect(prLook(pr({ checks: { total: 3, pass: 2, fail: 1, pending: 0, skipped: 0 } }))).toMatchObject({ tone: "red", label: "Checks failed" });
    expect(prLook(pr({ review: "CHANGES_REQUESTED" }))).toMatchObject({ tone: "amber", label: "Changes requested" });
    expect(prLook(pr({ merge_state: "BLOCKED", review: "REVIEW_REQUIRED" }))).toMatchObject({ tone: "green", label: "Open", ready: false });
  });

  it("marks a request ready to merge only when GitHub says it merges now and no review stands in the way", () => {
    expect(prLook(pr({ merge_state: "CLEAN", review: "APPROVED" })).ready).toBe(true);
    expect(prLook(pr({ merge_state: "CLEAN" })).ready).toBe(true);
    expect(prLook(pr({ merge_state: "CLEAN", review: "REVIEW_REQUIRED" })).ready).toBe(false);
    expect(prLook(pr({ merge_state: "CLEAN", threads: 2 })).ready).toBe(false);
    expect(prLook(pr({ merge_state: "BEHIND", review: "APPROVED" })).ready).toBe(false);
  });

  it("says a request not read yet is being read, and one gh could not read is unknown", () => {
    expect(prLook(pr(null))).toMatchObject({ tone: "none", label: "being read…" });
    expect(prLook({ ...pr(null), error: "sh: gh: command not found" })).toMatchObject({ tone: "none", label: "state unknown" });
  });
});

describe("a pull request's words", () => {
  it("numbers it the way its site does and names its repository", () => {
    expect(prNumber(pr())).toBe("#212");
    const mr: PullRequest = { url: "https://gitlab.example.com/team/sub/api/-/merge_requests/9", label: "MR !9" };
    expect(prNumber(mr)).toBe("!9");
    expect(prRepo(mr.url)).toBe("team/sub/api");
    expect(prRepo("https://github.com/acme/storefront/pull/212")).toBe("acme/storefront");
    expect(prRepo("https://example.com/x")).toBe("example.com");
    expect(prHost(mr.url)).toBe("GitLab");
    expect(prHost(pr().url)).toBe("GitHub");
  });

  it("tips the badge with where it is, how it stands, its title and how to open it on its site", () => {
    const named = pr({ title: "Add saved carts for signed-in shoppers" });
    expect(prTip(named, true)).toBe("acme/storefront#212 · Open\nAdd saved carts for signed-in shoppers\n⌘-click opens it on GitHub");
    expect(prTip(named, false).split("\n")[2]).toBe("Ctrl-click opens it on GitHub");
    expect(prTip({ ...pr(null), error: "gh: not signed in" }, true)).toContain("Couldn't read it: gh: not signed in");
  });

  it("reads ⌘ as the link's click on a Mac and Ctrl or ⌘ elsewhere", () => {
    expect(isLinkClick({ metaKey: true, ctrlKey: false }, true)).toBe(true);
    expect(isLinkClick({ metaKey: false, ctrlKey: true }, true)).toBe(false);
    expect(isLinkClick({ metaKey: false, ctrlKey: true }, false)).toBe(true);
    expect(isLinkClick({ metaKey: false, ctrlKey: false }, false)).toBe(false);
  });
});

describe("the requests on a card", () => {
  const pane = (id: string, url?: string, agent = "claude") => ({ id, title: "", agent, cwd: "/w", focused: false, status: "idle" as const, cols: 80, rows: 24, ...(url ? { pr: { url, label: `PR #${url.split("/").pop()}` } } : {}) });
  const session = (panes: ReturnType<typeof pane>[]): Session => ({ id: "w1", name: "app", status: "idle", active_tab_id: "t1", tabs: [{ id: "t1", label: "1", number: 1, named: false, status: "idle", panes }] });

  it("are each request its agents named once, the lead's first, a shell's never", () => {
    const s = session([pane("p1", "https://github.com/a/b/pull/1"), pane("p2", "https://github.com/a/b/pull/2"), pane("p3", "https://github.com/a/b/pull/1"), pane("p4", "https://github.com/a/b/pull/4", "")]);
    expect(cardPRs(s).map((p) => p.pr.url)).toEqual(["https://github.com/a/b/pull/1", "https://github.com/a/b/pull/2"]);
    expect(cardPRs(s, "p2").map((p) => p.paneId)).toEqual(["p2", "p1"]);
    expect(moreTip(cardPRs(s).slice(1))).toBe("a/b#2 · PR #2");
  });
});
