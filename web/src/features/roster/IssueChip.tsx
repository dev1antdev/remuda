// IssueChip is a Jira issue a workspace is about, "CRMOPS-6279 · In Review" in its status's colour — the key never cut,
// the status before it — and a click opens it in Jira.

import type { MouseEvent } from "react";
import { issueTitle, issueTone, issueWords } from "@/features/jira/jira";
import { openUrl } from "@/lib/host";
import type { JiraIssue } from "./api";

export function IssueChip({ issue, className = "" }: { issue: JiraIssue; className?: string }) {
  const open = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    openUrl(issue.url);
  };
  return (
    <button type="button" className={`linkish jira-chip tone-${issueTone(issue.category)}${className ? ` ${className}` : ""}`} title={issueTitle(issue)} aria-label={issueWords(issue)} onClick={open}>
      <i className="jira-dot" aria-hidden="true" />
      <span className="jira-key">{issue.key}</span>
      {issue.status && <span className="jira-status">· {issue.status}</span>}
    </button>
  );
}
