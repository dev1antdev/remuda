// The Russian words of a workspace's worktree: the new workspace a repository folder makes, the worktree a close takes
// down after it, and the leftover worktrees' dialog.

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
    "Worktree в {root}, которые не открыты ни в одном пространстве. Чистый удаляется вместе с веткой, если она слита; с незакоммиченными изменениями — остаётся.",
  "Looking at the worktrees…": "Смотрю worktree…",
  "Nothing left behind: every worktree here has its workspace.": "Ничего не осталось: у каждого worktree здесь есть своё пространство.",
  "Removing…": "Удаляю…",
  "Remove {worktrees} · {size}": "Удалить {worktrees} · {size}",
  "Leftover worktrees…": "Оставшиеся worktree…",
  "worktrees no workspace has open": "worktree без открытого пространства",
  "Leftover worktrees on {machine}…": "Оставшиеся worktree на {machine}…",
  "New workspace…": "Новое пространство…",
  "a worktree of its own: a new folder and branch, an agent in it": "свой worktree: новая папка и ветка, в ней агент",
  "New workspace in {repo}": "Новое пространство в {repo}",
  "A worktree of its own: a folder with its own branch beside {repo}, so the agents of different workspaces never touch each other's files. Closing the workspace removes it again once nothing in it is uncommitted.":
    "Свой worktree: папка со своей веткой рядом с {repo}, чтобы агенты разных пространств не трогали файлы друг друга. При закрытии пространства он удаляется, если в нём нет незакоммиченных изменений.",
  "Create workspace": "Создать пространство",
  "Base (optional — a branch, tag or commit; the current HEAD by default)": "Основа (необязательно — ветка, тег или коммит; по умолчанию текущий HEAD)",
  "Branch name is required": "Нужно имя ветки",
  "New workspace in {workspace}…": "Новое пространство в {workspace}…",
};
