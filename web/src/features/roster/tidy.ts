// What a workspace's close does to the worktree it stood on, and the worktrees closes left behind: the words for the
// close's confirm and its toast, and for the rows of the leftovers dialog. A worktree herdr or remuda made sits under
// ~/.herdr/worktrees/<repository>/<name>; any other folder a workspace stands on is never touched.

import { basename, plural } from "@/lib/text";
import { t } from "@/lib/i18n";

// Outcome is what became of a worktree: removed or kept with the reason, its branch deleted or kept with git's line
export type Outcome = { path: string; branch?: string; removed: boolean; branch_deleted: boolean; kept?: string; branch_kept?: string };

// Leftover is a worktree under the machine's root that no workspace has open
export type Leftover = { path: string; repo: string; name: string; branch?: string; dirty?: string; broken?: string; kb: number };

const MADE = /\/\.herdr\/worktrees\/[^/]+\/[^/]/;

// madeWorktree says whether a workspace stands on a worktree herdr or remuda made, the only kind a close takes down
export const madeWorktree = (worktree: { linked: boolean; checkout_path: string } | undefined): boolean => !!worktree?.linked && MADE.test(worktree.checkout_path);

// closeAsk is the close's question, with what happens to the worktree when there is one to take down
export function closeAsk(panes: string, made: boolean): string {
  if (!made) return panes;
  return `${panes} ${t("Its worktree folder is removed too, unless it has uncommitted changes, and its branch once merged.")}`;
}

// tidyWords is the toast after a close took a worktree down, or kept it
export function tidyWords(outcome: Outcome): { text: string; error: boolean } {
  const name = basename(outcome.path);
  if (!outcome.removed) return { text: t("The worktree {name} stays: {why}.", { name, why: outcome.kept ?? t("git refused") }), error: true };
  if (outcome.branch_deleted) return { text: t("The worktree {name} is removed, its branch {branch} too.", { name, branch: outcome.branch ?? name }), error: false };
  if (outcome.branch && outcome.branch_kept) return { text: t("The worktree {name} is removed; its branch {branch} stays, not merged yet.", { name, branch: outcome.branch }), error: false };
  return { text: t("The worktree {name} is removed.", { name }), error: false };
}

// removable is a leftover nothing would be lost from: git knows it and nothing is uncommitted
export const removable = (left: Leftover): boolean => !left.broken && !left.dirty;

// sizeWords is a folder's size as the dialog says it
export function sizeWords(kb: number): string {
  if (kb < 1024) return t("{n} KB", { n: kb });
  if (kb < 1024 * 1024) return t("{n} MB", { n: (kb / 1024).toFixed(kb < 10 * 1024 ? 1 : 0) });
  return t("{n} GB", { n: (kb / 1024 / 1024).toFixed(1) });
}

// leftoverWords is a row's state: its branch and size, or why it cannot go
export function leftoverWords(left: Leftover): string {
  if (left.broken) return left.broken;
  if (left.dirty) return t("uncommitted: {change}", { change: left.dirty });
  return [left.branch, sizeWords(left.kb)].filter(Boolean).join(" · ");
}

// removedWords sums up a removal: how many went, how many stayed and the first reason
export function removedWords(outcomes: Outcome[]): { text: string; error: boolean } {
  const gone = outcomes.filter((o) => o.removed).length;
  const kept = outcomes.filter((o) => !o.removed);
  if (kept.length === 0) return { text: t("Removed {worktrees}.", { worktrees: plural(gone, "worktree") }), error: false };
  const why = `${basename(kept[0]!.path)}: ${kept[0]!.kept ?? t("git refused")}`;
  return { text: t("Removed {worktrees}; {kept} stayed — {why}.", { worktrees: plural(gone, "worktree"), kept: kept.length, why }), error: true };
}
