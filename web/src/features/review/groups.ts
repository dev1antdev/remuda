// The Changes lens's file list as git sees it, in better-changes' order: the conflicts, what is staged, what is not and
// what git does not track yet — a file staged and changed again in both groups, each side with its own counts — and
// what each row offers: stage, unstage, discard, each discard in words that say what goes.

import { t, tn } from "@/lib/i18n";
import { basename } from "@/lib/text";
import type { DiffFrom, DiscardKind, ReviewFile, Stat } from "./api";

export type GroupId = "conflict" | "staged" | "unstaged" | "untracked";
export const GROUPS: GroupId[] = ["conflict", "staged", "unstaged", "untracked"];

// ChangeRow is one file in one group: its key (a file in two groups has two rows), the counts of that side, git's
// letter for it, and which diff it opens on
export type ChangeRow = { key: string; group: GroupId; file: ReviewFile; stat: Stat | null; letter: string; from: DiffFrom };

export const rowKey = (group: string, path: string) => `${group}:${path}`;
export const pathOfKey = (key: string) => key.slice(key.indexOf(":") + 1);

// rowsOf lays the files out in their groups, in the order the summary gives them (the fresh ones first). A file partly
// staged opens each side on its own diff; any other keeps the lens's usual one, the edits since the last look
export function rowsOf(files: ReviewFile[]): Record<GroupId, ChangeRow[]> {
  const groups: Record<GroupId, ChangeRow[]> = { conflict: [], staged: [], unstaged: [], untracked: [] };
  for (const file of files) {
    const partly = !!file.staged && !!file.unstaged;
    if (file.conflict) groups.conflict.push({ key: rowKey("conflict", file.path), group: "conflict", file, stat: null, letter: "U", from: {} });
    else if (file.untracked) groups.untracked.push({ key: rowKey("untracked", file.path), group: "untracked", file, stat: null, letter: "", from: {} });
    else if (!file.staged && !file.unstaged) groups.unstaged.push({ key: rowKey("unstaged", file.path), group: "unstaged", file, stat: { added: file.added, removed: file.removed }, letter: "M", from: {} }); // a summary from before git's status was read
    else {
      if (file.staged) groups.staged.push({ key: rowKey("staged", file.path), group: "staged", file, stat: file.staged, letter: file.index || "M", from: partly ? { side: "staged" } : {} });
      if (file.unstaged) groups.unstaged.push({ key: rowKey("unstaged", file.path), group: "unstaged", file, stat: file.unstaged, letter: file.work || "M", from: partly ? { side: "unstaged" } : {} });
    }
  }
  return groups;
}

// shownOrder is every row in the order the lens draws them, the groups one after another
export const shownOrder = (groups: Record<GroupId, ChangeRow[]>): ChangeRow[] => GROUPS.flatMap((group) => groups[group]);

// rowFor is the row a path opens on when nothing says which: where the working tree's text is, the unstaged side first
export function rowFor(groups: Record<GroupId, ChangeRow[]>, path: string): ChangeRow | undefined {
  for (const group of ["unstaged", "untracked", "staged", "conflict"] as GroupId[]) {
    const row = groups[group].find((candidate) => candidate.file.path === path);
    if (row) return row;
  }
  return undefined;
}

export const groupTitle = (group: GroupId): string =>
  ({ conflict: t("Conflicts"), staged: t("Staged"), unstaged: t("Unstaged"), untracked: t("Untracked") })[group];

// BulkAction is a group's one button: everything unstaged or untracked staged, everything staged unstaged
export type BulkAction = "stage" | "unstage";
export const bulkOf = (group: GroupId): BulkAction | null => (group === "staged" ? "unstage" : group === "unstaged" || group === "untracked" ? "stage" : null);
export const bulkWords = (action: BulkAction): string => (action === "stage" ? t("Stage all") : t("Unstage all"));

// RowAction is what a row's buttons do in place of its counts: a conflict is staged once resolved, never discarded
export type RowAction = "stage" | "unstage" | "discard";
export function rowActions(row: ChangeRow): RowAction[] {
  if (row.group === "conflict") return ["stage"];
  if (row.group === "staged") return ["unstage", "discard"];
  return ["stage", "discard"];
}

// discardKind is what a row's discard throws away: the unstaged edits back to what is staged, a staged file back to the
// last commit (a new one deleted), an untracked file deleted
export const discardKind = (row: ChangeRow): DiscardKind => (row.group === "untracked" ? "untracked" : row.group === "staged" ? "all" : "unstaged");

// discardAsk is the confirm before a discard, naming the file and what cannot come back
export function discardAsk(row: ChangeRow): { text: string; button: string; done: string } {
  const name = row.file.path;
  const deleted = { button: t("Delete the file"), done: t("Deleted {file}.", { file: name }) };
  const discarded = { button: t("Discard"), done: t("Discarded the changes to {file}.", { file: name }) };
  if (row.group === "untracked") return { text: t("Delete {file}? It was never committed, so it cannot be brought back.", { file: name }), ...deleted };
  if (row.group === "staged" && row.file.index === "A") return { text: t("Delete {file}? It is new and was never committed, so it cannot be brought back.", { file: name }), ...deleted };
  if (row.group === "staged") return { text: t("Discard every change to {file}, staged and not? It goes back to how the last commit has it, and the changes cannot be brought back.", { file: name }), ...discarded };
  if (row.file.staged) return { text: t("Discard the unstaged changes to {file}? What is staged stays; the rest cannot be brought back.", { file: name }), ...discarded };
  return { text: t("Discard the changes to {file}? It goes back to how the last commit has it, and the changes cannot be brought back.", { file: name }), ...discarded };
}

// actionWords is an action's name on its button, for the reader and the tooltip
export const actionWords = (action: RowAction, row: ChangeRow): string =>
  action === "stage" ? (row.group === "conflict" ? t("Stage {file} to mark it resolved", { file: basename(row.file.path) }) : t("Stage {file}", { file: basename(row.file.path) })) : action === "unstage" ? t("Unstage {file}", { file: basename(row.file.path) }) : t("Discard the changes to {file}…", { file: basename(row.file.path) });

// CONFLICT_KINDS is what an unmerged file's letters mean: yours first, then the incoming side
const CONFLICT_KINDS: Record<string, string> = {
  UU: "both edited",
  AA: "both added",
  DD: "both deleted",
  DU: "deleted by you",
  UD: "deleted incoming",
  AU: "added by you",
  UA: "added incoming",
};
export const conflictWords = (letters: string | undefined): string => t(CONFLICT_KINDS[letters ?? ""] ?? "in conflict");

// commitAllPaths is what Commit all stages first: every file listed unstaged or untracked, the conflicts left out
export const commitAllPaths = (groups: Record<GroupId, ChangeRow[]>): string[] => [...new Set([...groups.unstaged, ...groups.untracked].map((row) => row.file.path))];

// bulkPaths is a group's files for its one button
export const bulkPaths = (rows: ChangeRow[]): string[] => rows.map((row) => row.file.path);

// wroteWords is the toast after a write: how many files moved, and the conflicts a bulk action passed over
export function wroteWords(action: BulkAction, done: { paths: string[]; skipped?: string[] }): string {
  const count = done.paths.length;
  const moved = count === 1 ? (action === "stage" ? t("Staged {file}.", { file: done.paths[0]! }) : t("Unstaged {file}.", { file: done.paths[0]! })) : action === "stage" ? tn(count, "Staged {n} file.", "Staged {n} files.") : tn(count, "Unstaged {n} file.", "Unstaged {n} files.");
  const skipped = done.skipped ?? [];
  return skipped.length > 0 ? `${moved} ${skipped.length === 1 ? t("{file} is in conflict and was left as it is.", { file: skipped[0]! }) : tn(skipped.length, "{n} file in conflict was left as it is.", "{n} files in conflict were left as they are.")}` : moved;
}
