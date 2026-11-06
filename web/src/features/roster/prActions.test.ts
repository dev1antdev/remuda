import { describe, expect, it } from "vitest";
import type { PRStatus, PullRequest } from "./api";
import { actionLabel, actionsFor, canHandFailure, canRemoveWorktree, canSendReview, handedWords, removeAsk, removedWords } from "./prActions";

const status = (over: Omit<Partial<PRStatus>, "checks"> & { checks?: Partial<PRStatus["checks"]> } = {}): PRStatus => ({
  state: "OPEN",
  draft: false,
  mergeable: "MERGEABLE",
  review: "",
  at: "2026-10-29T12:00:00Z",
  ...over,
  checks: { total: 0, pass: 0, fail: 0, pending: 0, skipped: 0, ...over.checks },
});

const pr = (s?: PRStatus): PullRequest => ({ url: "https://github.com/o/r/pull/15", label: "PR #15", status: s });

describe("what the request's state allows", () => {
  it("hands a failure only while a check fails on an open request", () => {
    expect(canHandFailure(undefined)).toBe(false);
    expect(canHandFailure(status())).toBe(false);
    expect(canHandFailure(status({ checks: { total: 3, pass: 2, fail: 1 } }))).toBe(true);
    expect(canHandFailure(status({ state: "MERGED", checks: { total: 3, fail: 1 } }))).toBe(false);
    expect(canHandFailure(status({ checks: { total: 2, pending: 2 } }))).toBe(false);
  });

  it("sends the review while the request is open, whatever the reviewers said so far", () => {
    expect(canSendReview(undefined)).toBe(false);
    expect(canSendReview(status())).toBe(true);
    expect(canSendReview(status({ review: "CHANGES_REQUESTED" }))).toBe(true);
    expect(canSendReview(status({ state: "CLOSED" }))).toBe(false);
    expect(canSendReview(status({ state: "MERGED" }))).toBe(false);
  });

  it("lets a linked worktree go once its request is merged", () => {
    const linked = { worktree: { repo_key: "k", repo_name: "remuda", linked: true, checkout_path: "/w" } };
    expect(canRemoveWorktree(linked, pr(status({ state: "MERGED" })))).toBe(true);
    expect(canRemoveWorktree(linked, pr(status()))).toBe(false);
    expect(canRemoveWorktree(linked, undefined)).toBe(false);
    expect(canRemoveWorktree({ worktree: { ...linked.worktree, linked: false } }, pr(status({ state: "MERGED" })))).toBe(false);
    expect(canRemoveWorktree({}, pr(status({ state: "MERGED" })))).toBe(false);
  });

  it("lists the hand-offs in the menu's order", () => {
    expect(actionsFor(undefined)).toEqual([]);
    expect(actionsFor(pr())).toEqual([]);
    expect(actionsFor(pr(status()))).toEqual(["review"]);
    expect(actionsFor(pr(status({ checks: { total: 1, fail: 1 } })))).toEqual(["failure", "review"]);
    expect(actionsFor(pr(status({ state: "MERGED", checks: { total: 1, fail: 1 } })))).toEqual([]);
  });

  it("says each row and each toast with the request's label", () => {
    expect(actionLabel("failure", "PR #15")).toBe("Hand the failed checks of PR #15 to the agent");
    expect(actionLabel("review", "PR #15")).toBe("Send the review comments of PR #15 to the agent");
    expect(handedWords("failure", "PR #15")).toBe("The failed checks of PR #15 are with the agent");
    expect(handedWords("review", "PR #15")).toBe("The review comments of PR #15 are with the agent");
    expect(removeAsk("task-20-x")).toContain("Closes “task-20-x”");
    expect(removeAsk("task-20-x")).toContain("uncommitted changes");
    expect(removedWords("task-20-x")).toBe("Worktree removed, branch task-20-x deleted");
    expect(removedWords("")).toBe("Worktree removed");
  });
});
