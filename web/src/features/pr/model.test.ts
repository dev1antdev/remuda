import { describe, expect, it } from "vitest";
import { actionQuestion, actions, blockers, cardChecks, checkCounts, checkGroups, kindOf, prLook, searchText, splitMessage, squashMessage, stats } from "./model";
import type { Check, Detail } from "./api";

const check = (name: string, over: Partial<Check> = {}): Check => ({ name, status: "COMPLETED", conclusion: "SUCCESS", ...over });

const sample = (over: Partial<Detail> = {}): Detail => ({
  id: "PR_1",
  number: 482,
  title: "Save trips for offline use",
  url: "https://github.com/octo/trips/pull/482",
  body: "Trips can be saved.",
  state: "OPEN",
  draft: false,
  author: "sam",
  head: "offline-trips",
  base: "main",
  head_oid: "0123456789abcdef0123456789abcdef01234567",
  changed_files: 3,
  additions: 120,
  deletions: 14,
  review_decision: "CHANGES_REQUESTED",
  mergeable: "MERGEABLE",
  merge_state: "BLOCKED",
  auto_merge: false,
  merge_methods: ["merge", "squash"],
  auto_allowed: true,
  repository: "octo/trips",
  viewer: "sam",
  comments: [],
  reviews: [],
  threads: [
    { id: "T1", path: "src/sync.ts", line: 42, resolved: false, outdated: false, can_resolve: true, can_reply: true, comments: [{ id: "C1", author: "priya", body: "Retry?", url: "u", at: "2026-11-13T10:00:00Z" }] },
    { id: "T2", path: "README.md", line: 3, resolved: true, outdated: false, can_resolve: true, can_reply: true, comments: [{ id: "C2", author: "noor", body: "Typo", url: "u", at: "2026-11-12T10:00:00Z" }] },
  ],
  checks: [
    check("unit", { conclusion: "FAILURE", run: 777, job: 1229, workflow: "CI" }),
    check("lint", { run: 777, job: 1230, workflow: "CI" }),
    check("e2e", { run: 778, job: 1300, workflow: "E2E", status: "IN_PROGRESS", conclusion: undefined }),
    check("ci/deploy", { status: "PENDING", conclusion: undefined }),
    check("docs", { run: 779, job: 1400, workflow: "Docs" }),
    check("skip", { run: 779, job: 1401, workflow: "Docs", conclusion: "SKIPPED" }),
  ],
  reviewers: [
    { login: "priya", state: "CHANGES_REQUESTED" },
    { login: "noor", state: "APPROVED" },
    { login: "lee", state: "REQUESTED" },
    { login: "kim", state: "COMMENTED" },
  ],
  ...over,
});
describe("checks", () => {
  it("reads a check by its conclusion, else its status", () => {
    expect(kindOf({ status: "COMPLETED", conclusion: "TIMED_OUT" })).toBe("failure");
    expect(kindOf({ status: "COMPLETED", conclusion: "CANCELLED" })).toBe("failure");
    expect(kindOf({ status: "in_progress" })).toBe("running");
    expect(kindOf({ status: "QUEUED" })).toBe("pending");
    expect(kindOf({ status: "EXPECTED" })).toBe("pending");
    expect(kindOf({ status: "COMPLETED", conclusion: "NEUTRAL" })).toBe("skipped");
    expect(cardChecks({ checks: "FAILURE" })).toBe("failure");
    expect(cardChecks({ checks: "" })).toBeNull();
  });
  it("counts them and puts the run that needs a look first", () => {
    const pr = sample();
    const { counts, worst } = checkCounts(pr.checks);
    expect(counts).toEqual({ failure: 1, running: 1, pending: 1, success: 2, skipped: 1 });
    expect(worst).toBe("failure");
    const groups = checkGroups(pr.checks);
    expect(groups.map((g) => g.name)).toEqual(["CI", "E2E", "Other checks", "Docs"]);
    expect(groups[0]!.checks[0]!.name).toBe("unit");
    expect(checkCounts([]).worst).toBeNull();
  });
});

describe("what stands in the way", () => {
  it("names the blockers, those someone must act on first", () => {
    expect(blockers(sample()).map((b) => b.label)).toEqual(["1 check failed", "Changes requested", "1 unresolved thread", "2 checks running"]);
    expect(blockers(sample({ draft: true, mergeable: "CONFLICTING", checks: [], threads: [], review_decision: "REVIEW_REQUIRED", merge_state: "BEHIND" })).map((b) => b.label)).toEqual(["Draft", "Conflicts", "Review required", "Behind main"]);
    expect(blockers(sample({ checks: [], threads: [], review_decision: "APPROVED", merge_state: "BLOCKED" })).map((b) => b.label)).toEqual(["Blocked by the branch's rules"]);
    expect(blockers(sample({ state: "MERGED" }))).toEqual([]);
  });
  it("says the stats in one line, always in the same order", () => {
    const line = stats(sample());
    expect(line.map((s) => [s.key, s.value, s.tone])).toEqual([
      ["checks", "2/5", "bad"],
      ["reviews", "1/3", "bad"],
      ["unresolved", "1", "bad"],
    ]);
    const ready = stats(sample({ checks: [check("unit")], threads: [], review_decision: "APPROVED", merge_state: "CLEAN", reviewers: [{ login: "noor", state: "APPROVED" }] }));
    expect(ready.map((s) => [s.key, s.value, s.tone])).toEqual([
      ["checks", "1/1", "good"],
      ["reviews", "1/1", "good"],
      ["unresolved", "0", "none"],
      ["merge", "Ready", "good"],
    ]);
    const none = stats(sample({ checks: [], reviewers: [], threads: [], review_decision: "", mergeable: "CONFLICTING" }));
    expect(none.map((s) => s.value)).toEqual(["–", "–", "0", "Conflicts"]);
  });
});

describe("actions", () => {
  it("offers what can be done, merge when ready only where GitHub waits", () => {
    expect(actions(sample())).toEqual(["auto", "merge", "squash", "close"]);
    expect(actions(sample({ merge_state: "CLEAN" }))).toEqual(["merge", "squash", "close"]);
    expect(actions(sample({ auto_allowed: false }))).toEqual(["merge", "squash", "close"]);
    expect(actions(sample({ auto_merge: true }))).toEqual(["cancel-auto", "close"]);
    expect(actions(sample({ draft: true }))).toEqual(["close"]);
    expect(actions(sample({ state: "CLOSED" }))).toEqual(["reopen"]);
    expect(actions(sample({ state: "MERGED" }))).toEqual([]);
  });
  it("asks in words and starts a squash with the title and description", () => {
    expect(actionQuestion("squash", sample())).toBe("Squash and merge #482 from offline-trips into main? This cannot be undone.");
    const message = squashMessage(sample({ body: "  Line one.\n\nTwo.  " }));
    expect(message).toBe("Save trips for offline use (#482)\n\nLine one.\n\nTwo.");
    expect(splitMessage(message)).toEqual({ subject: "Save trips for offline use (#482)", body: "Line one.\n\nTwo." });
    expect(splitMessage(" just a subject ")).toEqual({ subject: "just a subject", body: "" });
  });
});

describe("the look and the search", () => {
  it("reads a state as its icon and tone", () => {
    expect(prLook({ state: "MERGED", draft: false }).tone).toBe("merged");
    expect(prLook({ state: "OPEN", draft: true }).icon).toBe("git-pull-request-draft");
    expect(prLook({ state: "CLOSED", draft: false }).tone).toBe("bad");
  });
  it("searches from three characters, or a number of any length", () => {
    expect(searchText("ab")).toBe("");
    expect(searchText(" abc ")).toBe("abc");
    expect(searchText("#4")).toBe("#4");
    expect(searchText("7")).toBe("7");
  });
});
