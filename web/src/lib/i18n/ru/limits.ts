// The Russian words of the plan limits: the Usage panel's section, the pill's tooltip, the machine's ⓘ, the chat's
// notice on an agent a limit stopped, the sidebar's words for it, the webhook kind and the activity chip.

import type { Dictionary } from "../../i18n";

export const limits: Dictionary = {
  "5 h": "5 ч",
  week: "неделя",
  "{agent} {window}: {used} %, resets {time}": "{agent} {window}: {used} %, сброс {time}",
  "Plan limits": "Лимиты плана",
  "No plan limits read yet: Codex reports its own after its next answer.": "Лимиты плана ещё не прочитаны: Codex сообщит свои после следующего ответа.",
  "Read on {machine}, {ago}": "Прочитано на {machine}, {ago}",
  "resets {time}": "сброс {time}",
  "Claude Code hands its plan limits to its status line only. On {machines}, Updates in the machine's ⓘ offers “Show Claude Code's plan limits”: remuda then reads them before your status line runs.":
    "Claude Code отдаёт лимиты плана только своей строке состояния. На {machines} в обновлениях (ⓘ машины) есть «Show Claude Code's plan limits»: тогда remuda читает их перед вашей строкой состояния.",
  "Plan: {words}.": "План: {words}.",
  "Call it off": "Отменить",
  "Continue after the reset": "Продолжить после сброса",
  "The agent stopped on its plan limit {ago}.": "Агент остановился на лимите плана {ago}.",
  "At {time} remuda says “continue” to it, as you booked.": "В {time} remuda скажет ему «continue», как вы назначили.",
  "The limit resets {time}.": "Лимит сбросится {time}.",
  "When the limit resets is not known here: Codex's come from its files, Claude Code's through its status line.":
    "Когда сбросится лимит, здесь неизвестно: лимиты Codex читаются из его файлов, Claude Code — через его строку состояния.",
  "stopped on its plan limit · continues after the reset": "упёрся в лимит плана · продолжит после сброса",
  "stopped on its plan limit": "упёрся в лимит плана",
  "a plan limit": "лимит плана",
  "Claude's or Codex's 5-hour or weekly window at 80 % or reached, and what a booked continue did":
    "5-часовое или недельное окно Claude или Codex на 80 % или исчерпано, и что сделало назначенное продолжение",
  "Budgets & limits": "Бюджеты и лимиты",
};
