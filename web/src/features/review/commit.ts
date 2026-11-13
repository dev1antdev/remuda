// The commit box at the foot of the file list, after better-changes and gitview: Commit staged, or Commit all with
// nothing staged (the listed files staged first), or Amend; what the button says, and why it is off when it is.

import { t, tn } from "@/lib/i18n";
import type { CheckState, SecretScan } from "./secrets";
import { foundCount } from "./secrets";

export type CommitMode = "commit" | "amend";

// MAX_ROWS is how tall the message box grows before it scrolls
export const MAX_ROWS = 8;
export const boxRows = (message: string): number => Math.min(MAX_ROWS, Math.max(2, message.split("\n").length));

// CommitState is what the box knows when it draws: the message, the mode, what is staged and what Commit all would
// stage, the conflicts, the agent at work there, and for an amend the HEAD it was chosen on
export type CommitState = {
  message: string;
  mode: CommitMode;
  staged: number;
  toStage: number;
  conflicts: number;
  busy: string;
  amendHead: string | null;
  loadingAmend: boolean;
  working: boolean;
};

// commitLabel is the button's word: what it would do right now
export function commitLabel(state: CommitState): string {
  if (state.working) return state.mode === "amend" ? t("Amending…") : t("Committing…");
  if (state.mode === "amend") return t("Amend");
  return state.staged > 0 || state.toStage === 0 ? t("Commit staged") : t("Commit all");
}

// whyNotCommit is the reason the button is off, "" when it is on; the busy agent first, since no write lands then
export function whyNotCommit(state: CommitState): string {
  if (state.busy) return t("{agent} is working here: commit once it rests.", { agent: state.busy });
  if (state.conflicts > 0) return t("Resolve the conflicts and stage the files before committing.");
  if (state.mode === "amend") {
    if (state.loadingAmend) return t("Loading the latest commit…");
    if (!state.amendHead) return t("Choose Amend again: the latest commit could not be read.");
  } else if (state.staged === 0 && state.toStage === 0) return t("Nothing to commit.");
  if (!state.message.trim()) return t("Write a message first.");
  return "";
}

// commitHint is the quiet line under the box: what Commit all stages, what Amend rewrites, a pushed commit's warning
export function commitHint(state: CommitState, amendPushed: boolean, subfolder: boolean): string {
  if (state.mode === "amend") return amendPushed ? t("The latest commit is on the remote already: amending it means a force push to share it.") : t("Amend rewrites the latest commit with what is staged; what is not staged stays as it is.");
  if (state.staged === 0 && state.toStage > 0) return tn(state.toStage, "Stages the {n} listed file first.", "Stages the {n} listed files first.");
  if (subfolder && state.staged > 0) return t("The commit takes everything staged in the repository, not only this folder.");
  return "";
}

// loadAmend is the message an amend puts in the box: the last commit's, only into an empty box
export const loadAmend = (current: string, last: string): string | null => (current.trim() ? null : last);

// leaveAmend is the box once Commit is chosen again: the message Amend loaded taken back, unless it was edited
export const leaveAmend = (current: string, loaded: string | null): string => (loaded !== null && current === loaded ? "" : current);

// committedWords is the toast after a commit
export const committedWords = (short: string, subject: string, amended: boolean): string =>
  amended ? t("Amended {sha}: {subject}", { sha: short, subject }) : t("Committed {sha}: {subject}", { sha: short, subject });

// commitAnywayAsk is the confirm before committing past what the secret check found, or past a check that failed
export function commitAnywayAsk(state: CheckState, scan: SecretScan | null): string {
  if (state === "failed") return t("The secret check could not run, so nothing is known about secrets in this work. Commit it anyway?");
  return tn(foundCount(scan), "{n} thing in this work looks like a secret. Commit it anyway?", "{n} things in this work look like secrets. Commit them anyway?");
}
