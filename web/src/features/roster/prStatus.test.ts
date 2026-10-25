import { describe, expect, it } from "vitest";
import type { PRStatus } from "./api";
import { checksTone, checksWords, prWords, reviewTone, reviewWords, statusTone } from "./prStatus";

const status = (over: Omit<Partial<PRStatus>, "checks"> & { checks?: Partial<PRStatus["checks"]> } = {}): PRStatus => ({
  state: "OPEN",
  draft: false,
  mergeable: "MERGEABLE",
  review: "",
  at: "2026-10-25T12:00:00Z",
  ...over,
  checks: { total: 0, pass: 0, fail: 0, pending: 0, skipped: 0, ...over.checks },
});

describe("checksWords", () => {
  it("counts the checks that ran, skipped ones aside", () => {
    expect(checksWords(status({ checks: { total: 3, pass: 2, skipped: 1 } }))).toBe("2/2 checks");
    expect(checksWords(status({ checks: { total: 5, pass: 5 } }))).toBe("5/5 checks");
  });
  it("says failures first, then what still runs", () => {
    expect(checksWords(status({ checks: { total: 6, pass: 2, fail: 1, pending: 2, skipped: 1 } }))).toBe("1 failed");
    expect(checksWords(status({ checks: { total: 4, pass: 1, pending: 3 } }))).toBe("3 running");
  });
  it("says when there are none", () => {
    expect(checksWords(status())).toBe("no checks");
    expect(checksWords(status({ checks: { total: 1, skipped: 1 } }))).toBe("no checks");
  });
});

describe("reviewWords", () => {
  it("names the final states first", () => {
    expect(reviewWords(status({ state: "MERGED", review: "APPROVED" }))).toBe("merged");
    expect(reviewWords(status({ state: "CLOSED" }))).toBe("closed");
    expect(reviewWords(status({ draft: true, review: "APPROVED" }))).toBe("draft");
    expect(reviewWords(status({ mergeable: "CONFLICTING", review: "APPROVED" }))).toBe("conflicts");
  });
  it("then the review's verdict", () => {
    expect(reviewWords(status({ review: "APPROVED" }))).toBe("approved");
    expect(reviewWords(status({ review: "CHANGES_REQUESTED" }))).toBe("changes requested");
    expect(reviewWords(status({ review: "REVIEW_REQUIRED" }))).toBe("review pending");
    expect(reviewWords(status())).toBe("");
  });
});

describe("checksTone", () => {
  it("follows the checks alone", () => {
    expect(checksTone(status({ checks: { total: 2, pass: 1, fail: 1 } }))).toBe("red");
    expect(checksTone(status({ checks: { total: 2, pass: 1, pending: 1 } }))).toBe("amber");
    expect(checksTone(status({ checks: { total: 3, pass: 2, skipped: 1 } }))).toBe("green");
    expect(checksTone(status({ review: "CHANGES_REQUESTED", checks: { total: 2, pass: 2 } }))).toBe("green");
    expect(checksTone(status())).toBe("none");
    expect(checksTone(status({ state: "MERGED", checks: { total: 2, fail: 2 } }))).toBe("dim");
  });
});

describe("reviewTone", () => {
  it("follows the review and the branch alone", () => {
    expect(reviewTone(status({ review: "APPROVED", checks: { total: 2, fail: 2 } }))).toBe("green");
    expect(reviewTone(status({ review: "CHANGES_REQUESTED" }))).toBe("red");
    expect(reviewTone(status({ mergeable: "CONFLICTING", review: "APPROVED" }))).toBe("red");
    expect(reviewTone(status({ review: "REVIEW_REQUIRED" }))).toBe("dim");
    expect(reviewTone(status({ draft: true, review: "APPROVED" }))).toBe("dim");
    expect(reviewTone(status({ state: "CLOSED", review: "APPROVED" }))).toBe("dim");
    expect(reviewTone(status())).toBe("none");
  });
});

describe("statusTone", () => {
  it("is the graver of the two chips: red for a failure, conflicts or changes requested", () => {
    expect(statusTone(status({ review: "APPROVED", checks: { total: 2, pass: 1, fail: 1 } }))).toBe("red");
    expect(statusTone(status({ mergeable: "CONFLICTING", checks: { total: 2, pass: 2 } }))).toBe("red");
    expect(statusTone(status({ review: "CHANGES_REQUESTED", checks: { total: 2, pass: 2 } }))).toBe("red");
  });
  it("is amber while checks run, dim for a draft or once settled", () => {
    expect(statusTone(status({ review: "APPROVED", checks: { total: 2, pass: 1, pending: 1 } }))).toBe("amber");
    expect(statusTone(status({ draft: true, checks: { total: 2, pass: 2 } }))).toBe("green");
    expect(statusTone(status({ draft: true }))).toBe("dim");
    expect(statusTone(status({ state: "MERGED", checks: { total: 2, fail: 2 } }))).toBe("dim");
    expect(statusTone(status({ state: "CLOSED" }))).toBe("dim");
  });
  it("is green when every check passed or the review approved, none when nothing is known", () => {
    expect(statusTone(status({ checks: { total: 3, pass: 2, skipped: 1 } }))).toBe("green");
    expect(statusTone(status({ review: "APPROVED" }))).toBe("green");
    expect(statusTone(status())).toBe("none");
    expect(statusTone(status({ mergeable: "UNKNOWN", checks: { total: 1, skipped: 1 } }))).toBe("none");
  });
});

describe("prWords", () => {
  it("joins the checks and the review when there is one", () => {
    expect(prWords(status({ review: "APPROVED", checks: { total: 2, pass: 2 } }))).toBe("2/2 checks · approved");
    expect(prWords(status({ checks: { total: 2, pass: 2 } }))).toBe("2/2 checks");
  });
});
