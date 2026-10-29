// The Russian words of the pull request's menu: the hand-offs to the agent and a merged worktree's removal.

import type { Dictionary } from "../../i18n";

export const pr: Dictionary = {
  "Closes “{name}” — its agent stops — then removes the worktree and deletes its branch from the repository, on the machine. A worktree with uncommitted changes is refused before anything closes.":
    "Закрывает «{name}» — его агент остановится — затем удаляет worktree и его ветку из репозитория, на самой машине. Worktree с незакоммиченными изменениями отклоняется до того, как что-либо закроется.",
  "Hand the failed checks of {pr} to the agent": "Отдать агенту упавшие проверки {pr}",
  "Open {pr}": "Открыть {pr}",
  "Reading the failed checks of {pr}…": "Читаю упавшие проверки {pr}…",
  "Reading the reviews of {pr}…": "Читаю ревью {pr}…",
  "Remove the worktree": "Удалить worktree",
  "Remove the worktree and its branch…": "Удалить worktree и его ветку…",
  "Send the review comments of {pr} to the agent": "Отправить агенту комментарии ревью {pr}",
  "The failed checks of {pr} are with the agent": "Упавшие проверки {pr} переданы агенту",
  "The review comments of {pr} are with the agent": "Комментарии ревью {pr} переданы агенту",
  "Worktree removed": "Worktree удалён",
  "Worktree removed, branch {branch} deleted": "Worktree удалён, ветка {branch} удалена",
  "{pr}: {status}. Open it, or hand its checks or review to the agent": "{pr}: {status}. Открыть, или отдать агенту его проверки или ревью",
  "{pr} actions": "Действия с {pr}",
};
