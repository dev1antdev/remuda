// The Russian words of the arena: one prompt to several agents side by side, its dialog and its panel.

import type { Dictionary } from "../../i18n";

export const arena: Dictionary = {
  "Arena for {task}": "Арена для {task}",
  "New arena": "Новая арена",
  "Every agent gets the same prompt in a worktree of its own, branched off the repository you pick, so their work never meets. Compare them in the Arenas panel and keep the best; the rest are closed with their work saved as patches.":
    "Каждый агент получает один и тот же промпт в собственном worktree от выбранного репозитория, и их работа не пересекается. Сравните их в панели «Арены» и оставьте лучшего; остальные закроются, а их работа сохранится патчами.",
  "What every agent should do": "Что должен сделать каждый агент",
  "default model": "модель по умолчанию",
  "Remove this lane": "Убрать эту дорожку",
  "Another agent": "Ещё агент",
  "Start {n} agents": "Запустить агентов: {n}",
  "Write the prompt every agent gets": "Напишите промпт для всех агентов",
  "An arena needs at least two agents": "Арене нужно хотя бы два агента",
  "An arena has at most six agents": "На арене не больше шести агентов",
  "Lane {n} needs a machine and a repository": "Дорожке {n} нужны машина и репозиторий",
  "Type the model's id, or pick the default": "Введите id модели или выберите модель по умолчанию",
  "no changes yet": "изменений пока нет",
  Arenas: "Арены",
  "No arena yet. An arena gives one prompt to several agents, each in a worktree of its own, so you can compare their work and keep the best.":
    "Арен пока нет. Арена даёт один промпт нескольким агентам, каждому в своём worktree, чтобы сравнить их работу и оставить лучшую.",
  Forget: "Забыть",
  kept: "оставлен",
  "closed, its work saved": "закрыт, работа сохранена",
  "did not start": "не запустился",
  "starting…": "запускается…",
  "its pane is gone": "его панели больше нет",
  "Keep this one": "Оставить этого",
  "Keep this lane? No other lane is open.": "Оставить эту дорожку? Других открытых дорожек нет.",
  "Keep this lane and close the other {n}? Their work — commits and uncommitted changes — is saved as patches in remuda's state folder first; then their agents stop, and their worktrees and branches are removed.": [
    "Оставить эту дорожку и закрыть ещё {n}? Её работа — коммиты и незакоммиченные изменения — сначала сохранится патчем в папке состояния remuda; потом её агент остановится, а worktree и ветка удалятся.",
    "Оставить эту дорожку и закрыть ещё {n}? Их работа — коммиты и незакоммиченные изменения — сначала сохранится патчами в папке состояния remuda; потом их агенты остановятся, а worktree и ветки удалятся.",
    "Оставить эту дорожку и закрыть ещё {n}? Их работа — коммиты и незакоммиченные изменения — сначала сохранится патчами в папке состояния remuda; потом их агенты остановятся, а worktree и ветки удалятся.",
  ],
  "Arena: one prompt to several agents…": "Арена: один промпт нескольким агентам…",
  "each in a worktree of its own, their work side by side, the best kept": "каждому свой worktree, работа рядом, лучшая остаётся",
  "the lanes of every arena: how each agent stands, its changes, keep the best": "дорожки каждой арены: состояние агентов, их изменения, выбор лучшего",
  "The same prompt to several agents at once, each in a worktree of its own, to keep the best": "Тот же промпт сразу нескольким агентам, каждому в своём worktree, чтобы оставить лучшего",
  "Arena…": "Арена…",
};
