import { describe, expect, it } from "vitest";
import { isIssueURL, issueLabel, issueLink } from "./issue";

describe("isIssueURL", () => {
  it("takes a GitHub issue address, with a trailing slash, a fragment or a query", () => {
    expect(isIssueURL("https://github.com/cli/cli/issues/14814")).toBe(true);
    expect(isIssueURL("  https://github.com/cli/cli/issues/14814/  ")).toBe(true);
    expect(isIssueURL("https://github.com/cli/cli/issues/14814#issuecomment-1")).toBe(true);
    expect(isIssueURL("https://github.com/dev1antdev/remuda/issues/3?notification_referrer_id=x")).toBe(true);
  });
  it("refuses a pull request, another host, plain http and a number of zero", () => {
    expect(isIssueURL("https://github.com/dev1antdev/remuda/pull/71")).toBe(false);
    expect(isIssueURL("https://gitlab.com/a/b/issues/1")).toBe(false);
    expect(isIssueURL("http://github.com/a/b/issues/1")).toBe(false);
    expect(isIssueURL("https://github.com/a/b/issues/0")).toBe(false);
    expect(isIssueURL("")).toBe(false);
  });
});

describe("issueLink", () => {
  it("reads the issue the import put first in the description", () => {
    const html = '<p>Issue: <a href="https://github.com/cli/cli/issues/14814">https://github.com/cli/cli/issues/14814</a> · by babakks</p><p>Body</p>';
    expect(issueLink(html)).toBe("https://github.com/cli/cli/issues/14814");
  });
  it("is null for a description written by hand or naming another link", () => {
    expect(issueLink("<p>Fix the login page</p>")).toBeNull();
    expect(issueLink("")).toBeNull();
    expect(issueLink('<p>Issue: <a href="https://example.com/x">x</a></p>')).toBeNull();
    expect(issueLink('<p>See <a href="https://github.com/cli/cli/issues/1">this</a></p>')).toBeNull();
  });
});

describe("issueLabel", () => {
  it("is owner/repo#number, or the text itself when it is no issue", () => {
    expect(issueLabel("https://github.com/cli/cli/issues/14814")).toBe("cli/cli#14814");
    expect(issueLabel("https://example.com")).toBe("https://example.com");
  });
});
