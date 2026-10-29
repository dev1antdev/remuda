// What the page offers for the pull request an agent named, beyond its link: which hand-offs its state allows — the
// failed checks' log, the reviewers' comments — whether its worktree may go, and the words that follow each.

import type { PRStatus, PullRequest, Session } from "./api";
import { t } from "@/lib/i18n";

export type PRAction = "failure" | "review";

// canHandFailure: the request is open and a check failed
export const canHandFailure = (s?: PRStatus): boolean => !!s && s.state === "OPEN" && s.checks.fail > 0;

// canSendReview: the request is open, so a reviewer may have spoken (the server says when nobody has)
export const canSendReview = (s?: PRStatus): boolean => !!s && s.state === "OPEN";

// canRemoveWorktree: a linked worktree whose request gh read as merged
export const canRemoveWorktree = (session: Pick<Session, "worktree">, pr?: PullRequest): boolean => !!session.worktree?.linked && pr?.status?.state === "MERGED";

// actionsFor lists the hand-offs the request's state allows, in the menu's order
export function actionsFor(pr?: PullRequest): PRAction[] {
  const out: PRAction[] = [];
  if (canHandFailure(pr?.status)) out.push("failure");
  if (canSendReview(pr?.status)) out.push("review");
  return out;
}

// actionLabel is the menu row's words
export const actionLabel = (action: PRAction, label: string): string =>
  action === "failure" ? t("Hand the failed checks of {pr} to the agent", { pr: label }) : t("Send the review comments of {pr} to the agent", { pr: label });

// handedWords is the toast once the message went
export const handedWords = (action: PRAction, label: string): string =>
  action === "failure" ? t("The failed checks of {pr} are with the agent", { pr: label }) : t("The review comments of {pr} are with the agent", { pr: label });

// removeAsk is the confirm before a worktree goes
export const removeAsk = (name: string): string =>
  t("Closes “{name}” — its agent stops — then removes the worktree and deletes its branch from the repository, on the machine. A worktree with uncommitted changes is refused before anything closes.", { name });

// removedWords is the toast once it is gone
export const removedWords = (branch: string): string => (branch ? t("Worktree removed, branch {branch} deleted", { branch }) : t("Worktree removed"));
