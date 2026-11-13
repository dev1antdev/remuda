// The Git lens's words and colours: a commit's age as the date column says it, its moment in full, where HEAD stands,
// the lanes' colours from the theme's terminal sixteen in the order git-graph cycles them.

import { locale, t, tn } from "@/lib/i18n";

// dateWords is the date column: now, 12m, 3h, 6d, then Oct 3 this year and Oct 2025 before it
export function dateWords(seconds: number, nowMs = Date.now()): string {
  const age = nowMs / 1000 - seconds;
  if (age < 60) return t("now");
  if (age < 3600) return t("{n}m", { n: Math.round(age / 60) });
  if (age < 86400) return t("{n}h", { n: Math.round(age / 3600) });
  if (age < 86400 * 30) return t("{n}d", { n: Math.round(age / 86400) });
  const day = new Date(seconds * 1000);
  const sameYear = day.getFullYear() === new Date(nowMs).getFullYear();
  return new Intl.DateTimeFormat(locale(), sameYear ? { month: "short", day: "numeric" } : { month: "short", year: "numeric" }).format(day);
}

const pad2 = (n: number) => String(n).padStart(2, "0");

// stampWords is the moment in full: 2026-10-28 12:09
export function stampWords(seconds: number): string {
  const d = new Date(seconds * 1000);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

// whereWords is the current-branch chip: the branch, or the commit a detached HEAD stands on
export function whereWords(branch: string | undefined, head: string | undefined): string {
  if (branch) return branch;
  return head ? t("detached at {hash}", { hash: head.slice(0, 8) }) : "";
}

// the theme's terminal colours a lane takes in turn: blue, green, magenta, yellow, cyan, red, then their bright ones
const LANE_ORDER = [4, 2, 5, 3, 6, 1, 12, 10, 13, 11, 14, 9];

// laneColors picks the lanes' colours out of a theme's sixteen
export function laneColors(sixteen: readonly string[]): string[] {
  return LANE_ORDER.map((i) => sixteen[i] ?? sixteen[i % 8] ?? "currentColor");
}

// commitsWords is the header's count: 300 commits, 300+ commits while more follow
export function commitsWords(count: number, more: boolean): string {
  return more ? t("{n}+ commits", { n: count }) : tn(count, "{n} commit", "{n} commits");
}

// filesWords is a commit's files: 3 files, and the ones past the list counted
export const filesWords = (count: number) => tn(count, "{n} file", "{n} files");

// operationWords names an operation stopped in progress
export function operationWords(operation: string): string {
  switch (operation) {
    case "merge":
      return t("A merge");
    case "rebase":
      return t("A rebase");
    case "cherry-pick":
      return t("A cherry-pick");
    case "revert":
      return t("A revert");
  }
  return t("An operation");
}

// stoppedWords is the banner over the graph while a merge, rebase, cherry-pick or revert waits on conflicts
export function stoppedWords(operation: string, conflicts: number): string {
  const what = operationWords(operation);
  if (conflicts > 0) return tn(conflicts, "{what} stopped on a conflict in {n} file.", "{what} stopped on conflicts in {n} files.", { what });
  return t("{what} is in progress.", { what });
}
