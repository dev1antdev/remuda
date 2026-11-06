// The words and tones of a Jira chip, and what the person typed read as an issue key. Pure; IssueChip and the import
// dialog draw them.

import type { JiraIssue } from "@/features/roster/api";
import { t } from "@/lib/i18n";

export type IssueTone = "dim" | "blue" | "green";

// issueTone is the colour Jira gives a status's category: to do, in progress, done
export function issueTone(category: string): IssueTone {
  if (category === "done") return "green";
  if (category === "indeterminate") return "blue";
  return "dim";
}

// issueWords is what a chip says: "CRMOPS-6279 · In Review"
export const issueWords = (issue: JiraIssue): string => (issue.status ? `${issue.key} · ${issue.status}` : issue.key);

// issueTitle is a chip's tooltip: the key and summary, then the status and who has it
export function issueTitle(issue: JiraIssue): string {
  const who = issue.assignee ? t("assigned to {name}", { name: issue.assignee }) : t("unassigned");
  return `${issue.key} — ${issue.summary}\n${issue.status} · ${who}\n${t("Open in Jira")}`;
}

// branchNamesIssue says whether a branch is named after the issue, so the chip can stand in for it on a narrow line
export const branchNamesIssue = (branch: string, issue: JiraIssue | undefined): boolean => !!issue && new RegExp(`(^|[^A-Z0-9])${issue.key}($|[^0-9])`).test(branch.toUpperCase());

const KEY = /^[A-Z][A-Z0-9_]+-[1-9][0-9]{0,6}$/;
const BROWSE = /(?:\/browse\/|[?&]selectedIssue=)([A-Za-z][A-Za-z0-9_]+-[1-9][0-9]{0,6})\b/;

// keyOf reads what was typed — a key in any case, or an issue's address — as a key; "" for anything else
export function keyOf(typed: string): string {
  const text = typed.trim();
  const address = BROWSE.exec(text);
  const key = (address ? address[1]! : text).toUpperCase();
  return KEY.test(key) ? key : "";
}
