// The Russian words of the models' scorecard: the Usage panel's Models tab, its cells and tooltips, the palette's row.

import type { Dictionary } from "../../i18n";

export const scorecard: Dictionary = {
  "What the panel shows": "Что показывает панель",
  Spend: "Затраты",
  "Models scorecard": "Оценка моделей",
  "which model does your work best: finished runs, first-time approvals, merged pull requests, arenas kept, cost and time":
    "какая модель лучше справляется с вашей работой: завершённые прогоны, одобрения с первого раза, слитые pull request’ы, оставленные на арене, цена и время",
  "From remuda's own records: the agents started on tasks and in arenas, the verdicts you gave in the Changes lens on their worktrees, the pull requests they opened and whether those merged. A share becomes a percentage from {n} runs on; before that it says how many, and no best is marked.":
    "По собственным записям remuda: агенты, запущенные на задачах и на аренах, вердикты, которые вы вынесли в линзе изменений на их worktree, открытые ими pull request’ы и слиты ли они. Доля становится процентом начиная с {n} прогонов; до того показано, сколько из скольких, и лучший не отмечается.",
  "{n} verdicts of the period were given on folders no task run worked in: they count for no model.": [
    "{n} вердикт за период вынесен в папке, где не работал ни один прогон задачи: он не засчитан ни одной модели.",
    "{n} вердикта за период вынесены в папках, где не работал ни один прогон задачи: они не засчитаны ни одной модели.",
    "{n} вердиктов за период вынесены в папках, где не работал ни один прогон задачи: они не засчитаны ни одной модели.",
  ],
  "No agent was started on a task or in an arena in this period. Start one from a task, or run an arena, and its model shows here.":
    "За этот период ни один агент не запускался на задаче или на арене. Запустите агента из задачи или устройте арену — и его модель появится здесь.",
  "Approved first time": "Одобрены с первого раза",
  "PRs merged": "Слитые PR",
  "Kept in arenas": "Оставлены на арене",
  "$ per run": "$ за прогон",
  "$ per approved": "$ за одобренный",
  "Minutes per run": "Минут на прогон",
  "{n} runs": ["{n} прогон", "{n} прогона", "{n} прогонов"],
  "{name}, last at work {ago}": "{name}, последний раз в работе {ago}",
  "model not read": "модель не прочитана",
  "{count} of {of}": "{count} из {of}",
  "No run of this model was reviewed in the Changes lens yet": "Ни один прогон этой модели ещё не проверен в линзе изменений",
  "{n} approved at the first review": ["{n} одобрен с первого раза", "{n} одобрены с первого раза", "{n} одобрены с первого раза"],
  "{n} rejected before an approval": ["{n} отклонён до одобрения", "{n} отклонены до одобрения", "{n} отклонены до одобрения"],
  "{n} of them approved later": ["{n} из них одобрен позже", "{n} из них одобрены позже", "{n} из них одобрены позже"],
  "from {n} runs only": ["всего по {n} прогону", "всего по {n} прогонам", "всего по {n} прогонам"],
  "Its runs opened no pull request": "Её прогоны не открыли ни одного pull request’а",
  "{n} pull requests opened": ["{n} pull request открыт", "{n} pull request’а открыто", "{n} pull request’ов открыто"],
  "In no decided arena": "Ни в одной решённой арене",
  "{n} lanes in decided arenas": ["{n} дорожка в решённых аренах", "{n} дорожки в решённых аренах", "{n} дорожек в решённых аренах"],
  "Not priced here": "Здесь без цены",
  "What the reviewed runs cost, over the approved ones": "Сколько стоили проверенные прогоны, в расчёте на одобренные",
  "Try it on a past task…": "Попробовать на прошлой задаче…",
  "Try it on a past task: {task} in an arena, {first} beside {second}": "Попробовать на прошлой задаче: {task} на арене, {first} рядом с {second}",
  "Try it on a past task: {task} in an arena": "Попробовать на прошлой задаче: {task} на арене",
  "The best of the models with enough runs": "Лучшая из моделей с достаточным числом прогонов",
};
