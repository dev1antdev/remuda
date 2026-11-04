// The Russian words of the sidebar's menus: a folder's and a workspace's ⋯, a machine's state and vitals, the New
// workspace dialog, an empty machine, the tab strip's states, a pull request's state.

import type { Dictionary } from "../../i18n";

export const sidebarmenus: Dictionary = {
  "Folder actions": "Действия с папкой",
  "New workspace in this folder…": "Новое пространство в этой папке…",
  "New workspace…": "Новое пространство…",
  "New tab…": "Новая вкладка…",
  "New worktree…": "Новый worktree…",
  "Rename workspace…": "Переименовать пространство…",
  "Open in the editor": "Открыть в редакторе",
  "the Code mode on this folder": "режим Code на этой папке",
  "a branch of its own beside the checkout, an agent in it": "своя ветка рядом с основной копией, в ней агент",
  "another agent or a terminal in the same folder": "ещё один агент или терминал в той же папке",
  "No workspaces here yet.": "Пространств здесь пока нет.",
  connected: "подключена",
  unreachable: "недоступна",
  ready: "готов",
  shell: "терминал",
  "Disk free": "Свободно на диске",
  "{free} GB free · {used}% used": "свободно {free} ГБ · занято {used}%",
  "{label}: min {min}, max {max}, now {now}": "{label}: мин. {min}, макс. {max}, сейчас {now}",
  "Name (optional)": "Имя (необязательно)",
  "Project folder is required": "Нужна папка проекта",
  // the pull request's state, in a card's menu and a wall tile's chip
  "no checks": "проверок нет",
  merged: "влит",
  draft: "черновик",
  conflicts: "конфликты",
  approved: "одобрен",
  "changes requested": "просят правок",
  "review pending": "ждёт ревью",
};
