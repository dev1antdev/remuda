// The pane's place in the status bar, after Orca's my-status: the workspace's branch, its pull request's icon in the
// colour of its state, the issue its work names. The pure part.

import type { JiraIssue, Pane, PullRequest, Session } from "@/features/roster/api";
import { prWords } from "@/features/roster/prStatus";
import { t } from "@/lib/i18n";

export type PRTone = "green" | "mauve" | "red" | "dim";

// Place is what the bar says of where the pane on screen works
export type Place = { branch?: string; pr?: PullRequest; issue?: JiraIssue };

// placeOf is the workspace's branch, the pull request the pane's agent named — else the first one an agent of the
// workspace named — and the first issue its work names
export function placeOf(session: Session, pane: Pane): Place {
  const panes = session.tabs.flatMap((tab) => tab.panes);
  return { branch: session.branch, pr: pane.pr ?? panes.find((p) => p.pr)?.pr, issue: session.jira?.[0] };
}

// prTone is the request's state as Orca colours it: open green, a draft dim, merged mauve, closed red; dim while
// nothing is read of it
export function prTone(pr: PullRequest): PRTone {
  const s = pr.status;
  if (!s) return "dim";
  if (s.state === "MERGED") return "mauve";
  if (s.state === "CLOSED") return "red";
  if (s.draft) return "dim";
  return "green";
}

// prTip is the request's tooltip: its label and how it stands
export const prTip = (pr: PullRequest): string => (pr.status ? t("{label} · {state}", { label: pr.label, state: prWords(pr.status) }) : pr.label);
