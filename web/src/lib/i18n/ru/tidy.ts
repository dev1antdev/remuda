// The Russian words of a closed workspace's worktree taken down after it, and of the leftover worktrees' dialog.

import type { Dictionary } from "../../i18n";

export const tidy: Dictionary = {
  "Its worktree folder is removed too, unless it has uncommitted changes, and its branch once merged.": "Папка его worktree тоже удалится, если в ней нет незакоммиченных изменений, а ветка — если она слита.",
  "The worktree {name} stays: {why}.": "Worktree {name} остаётся: {why}.",
  "git refused": "git отказал",
  "The worktree {name} is removed, its branch {branch} too.": "Worktree {name} удалён, его ветка {branch} тоже.",
  "The worktree {name} is removed; its branch {branch} stays, not merged yet.": "Worktree {name} удалён; ветка {branch} осталась: она ещё не слита.",
  "The worktree {name} is removed.": "Worktree {name} удалён.",
  "{n} GB": "{n} ГБ",
  "uncommitted: {change}": "не закоммичено: {change}",
  "{n} worktrees": ["{n} worktree", "{n} worktree", "{n} worktree"],
  "Removed {worktrees}.": "Удалено: {worktrees}.",
  "Removed {worktrees}; {kept} stayed — {why}.": "Удалено: {worktrees}; осталось {kept} — {why}.",
  "Leftover worktrees on {machine}": "Оставшиеся worktree на {machine}",
  "Worktrees under {root} that no workspace has open. A clean one goes with its branch once merged; one with uncommitted changes stays.":
    "Worktree в {root}, которые не открыты ни в одном воркспейсе. Чистый удаляется вместе с веткой, если она слита; с незакоммиченными изменениями — остаётся.",
  "Looking at the worktrees…": "Смотрю worktree…",
  "Nothing left behind: every worktree here has its workspace.": "Ничего не осталось: у каждого worktree здесь есть свой воркспейс.",
  "Removing…": "Удаляю…",
  "Remove {worktrees} · {size}": "Удалить {worktrees} · {size}",
  "Leftover worktrees…": "Оставшиеся worktree…",
  "worktrees no workspace has open": "worktree без открытого воркспейса",
  "Leftover worktrees on {machine}…": "Оставшиеся worktree на {machine}…",
};
