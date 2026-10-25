// A task made from a GitHub issue: the address a person pastes, and the issue a description names in its first
// paragraph, as the import writes it (internal/tasks/issue.go, IssueHTML).

const ISSUE_RE = /^https:\/\/github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/issues\/([1-9][0-9]*)\/?(?:[?#].*)?$/;

// isIssueURL says whether a text is a GitHub issue's address (never a pull request's)
export const isIssueURL = (raw: string): boolean => ISSUE_RE.test(raw.trim());

// issueLink is the issue a description names in its first paragraph, or null for a description written by hand
export function issueLink(description: string): string | null {
  const m = /^\s*<p>Issue: <a href="(https:\/\/github\.com\/[^"]+)"/.exec(description);
  return m && isIssueURL(m[1]!) ? m[1]! : null;
}

// issueLabel is the short name of an issue, "owner/repo#123"
export function issueLabel(url: string): string {
  const m = ISSUE_RE.exec(url.trim());
  return m ? `${m[1]}/${m[2]}#${m[3]}` : url;
}
