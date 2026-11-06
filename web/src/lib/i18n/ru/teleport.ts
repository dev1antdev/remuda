// The Russian words of the move of an agent's work to another machine.

import type { Dictionary } from "../../i18n";

export const teleport: Dictionary = {
  "{words}: a new worktree of a clone there gets the branch, its commits and the uncommitted changes as they are; {agent} starts there, told where the work came from. Nothing here changes.": "{words}: новый worktree клона там получит ветку, её коммиты и незакоммиченные изменения как есть; {agent} запустится там и узнает, откуда пришла работа. Здесь ничего не меняется.",
  "a clone of this repository there": "клон этого репозитория там",
  "Pick a machine to move the work to": "Выберите машину, куда перенести работу",
  "Pick or type a clone of this repository on {machine}": "Выберите или впишите клон этого репозитория на {machine}",
  "The folder must be a full path, from /": "Папка должна быть полным путём, от /",
  "Pick the agent that carries on": "Выберите агента, который продолжит",
  "{n} clones of this repository": ["{n} клон этого репозитория", "{n} клона этого репозитория", "{n} клонов этого репозитория"],
  "no clone of this repository open": "клон этого репозитория там не открыт",
  "Close the tab of {workspace} here? Its agent stops; the work itself stays in its folder.": "Закрыть вкладку {workspace} здесь? Её агент остановится; сама работа останется в папке.",
  "Move the work to another machine": "Перенести работу на другую машину",
  "Reading the work in {workspace}…": "Читаю работу в {workspace}…",
  "{words}: a new worktree of a clone there gets the branch, its commits and the uncommitted changes as they are; {agent} carries on with the gist of this conversation. Nothing here changes.": "{words}: новый worktree клона там получит ветку, её коммиты и незакоммиченные изменения как есть; {agent} продолжит с выжимкой этого разговора. Здесь ничего не меняется.",
  "Clone of the repository there": "Клон репозитория там",
  "Moved: {agent} carries on on {machine}. The agent here keeps running until you close it.": "Перенесено: {agent} продолжает на {machine}. Агент здесь работает, пока вы его не закроете.",
  "The move stopped": "Перенос остановился",
  "Close the agent here…": "Закрыть агента здесь…",
  "Move to {machine}": "Перенести на {machine}",
  "Open it there": "Открыть там",
  "Move the work to another machine…": "Перенести работу на другую машину…",
  "Move the work in {workspace} to another machine…": "Перенести работу из {workspace} на другую машину…",
  "its branch, commits and uncommitted changes to a clone there, an agent carrying on": "её ветка, коммиты и незакоммиченные изменения — в клон там, и агент продолжает",
};
