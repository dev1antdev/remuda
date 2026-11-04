import { describe, expect, it } from "vitest";
import { branchNamesIssue, issueTitle, issueTone, issueWords, keyOf } from "./jira";

const issue = { key: "CRMOPS-6279", summary: "Conflicts with the base branch", status: "In Review", category: "indeterminate", assignee: "Sergii", url: "https://x/browse/CRMOPS-6279" };

describe("a Jira chip", () => {
  it("says the key and the status in the category's colour", () => {
    expect(issueWords(issue)).toBe("CRMOPS-6279 · In Review");
    expect(issueWords({ ...issue, status: "" })).toBe("CRMOPS-6279");
    expect(issueTone("indeterminate")).toBe("blue");
    expect(issueTone("done")).toBe("green");
    expect(issueTone("new")).toBe("dim");
    expect(issueTone("")).toBe("dim");
  });

  it("names the summary and who has it in its tooltip", () => {
    expect(issueTitle(issue)).toBe("CRMOPS-6279 — Conflicts with the base branch\nIn Review · assigned to Sergii\nOpen in Jira");
    expect(issueTitle({ ...issue, assignee: undefined })).toContain("unassigned");
  });
});

describe("branchNamesIssue", () => {
  it("lets the chip stand in for a branch named after its issue, in any case", () => {
    expect(branchNamesIssue("feature/crmops-6279-conflicts", issue)).toBe(true);
    expect(branchNamesIssue("CRMOPS-6279", issue)).toBe(true);
    expect(branchNamesIssue("feature/crmops-62790-x", issue)).toBe(false);
    expect(branchNamesIssue("xcrmops-6279", issue)).toBe(false);
    expect(branchNamesIssue("main", issue)).toBe(false);
    expect(branchNamesIssue("feature/crmops-6279", undefined)).toBe(false);
  });
});

describe("keyOf", () => {
  it("reads a key or an issue's address", () => {
    expect(keyOf("CRMOPS-6279")).toBe("CRMOPS-6279");
    expect(keyOf(" crmops-6279 ")).toBe("CRMOPS-6279");
    expect(keyOf("https://roofr.atlassian.net/browse/CRMOPS-6279")).toBe("CRMOPS-6279");
    expect(keyOf("https://roofr.atlassian.net/jira/software/projects/CRM/boards/1?selectedIssue=CRM-12")).toBe("CRM-12");
    for (const bad of ["", "6279", "CRMOPS", "CRMOPS-0", "X-1", "https://example.com"]) expect(keyOf(bad)).toBe("");
  });
});
