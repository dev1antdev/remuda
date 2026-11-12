import type { Dictionary } from "../../i18n";

// the words of a machine about to stop working quietly: Claude Code's sign-in running out there, and a model or mode
// being retired that a recipe, a schedule, a project's agents or a running agent is still set to
export const signin: Dictionary = {
  "Claude Code's sign-in on {machine} ended {ago}: its agents cannot reach Claude until you run claude auth login there":
    "Вход Claude Code на {machine} закончился {ago}: его агенты не смогут обратиться к Claude, пока вы не выполните там claude auth login",
  "Claude Code's sign-in on {machine} ends in {left}: run claude auth login there before then": "Вход Claude Code на {machine} закончится через {left}: выполните там claude auth login до этого",
  "ended {date}": "закончился {date}",
  "ends {date} · in {left}": "заканчивается {date} · через {left}",
  "Claude sign-in": "Вход в Claude",
  "Open a shell there": "Открыть там оболочку",
  "a sign-in running out": "заканчивающийся вход",
  "Claude Code's sign-in on a machine ends in three days, in a day, or has ended": "вход Claude Code на машине закончится через три дня, через день или уже закончился",

  "Fast mode on {model}": "Быстрый режим {model}",
  "{what} was removed on {date}: its requests fail now and do not fall back.": "{what} отключён {date}: его запросы теперь завершаются ошибкой, без перехода на запасной вариант.",
  "{what} ends on {date}: after that its requests fail and do not fall back.": "{what} отключат {date}: после этого его запросы будут завершаться ошибкой, без перехода на запасной вариант.",
  "Use {replacement}.": "Используйте {replacement}.",
  "{model} is the default model in Claude Code's settings on {machines}.": "{model} — модель по умолчанию в настройках Claude Code на {machines}.",
  "Fast mode is saved on in Claude Code's settings on {machines}.": "Быстрый режим включён в настройках Claude Code на {machines}.",
  "Use {model}": "Использовать {model}",
  "{n} recipes": ["{n} рецепт", "{n} рецепта", "{n} рецептов"],
  "{n} agent schedules": ["{n} расписание-разведчик", "{n} расписания-разведчика", "{n} расписаний-разведчиков"],
  "the agents of {n} projects": ["агенты {n} проекта", "агенты {n} проектов", "агенты {n} проектов"],
  "{n} pipeline stages": ["{n} этап конвейера", "{n} этапа конвейера", "{n} этапов конвейера"],
  "{n} running agents": ["{n} запущенный агент", "{n} запущенных агента", "{n} запущенных агентов"],
  "{what} ends on {date}, set for {set} here: move to {replacement}": "{what} отключат {date}, а здесь он задан для: {set} — переведите их на {replacement}",
  "{what} was removed on {date} and is still set for {set} here: their requests fail now — move to {replacement}":
    "{what} отключён {date}, а здесь он всё ещё задан для: {set} — их запросы теперь завершаются ошибкой; переведите их на {replacement}",
};
