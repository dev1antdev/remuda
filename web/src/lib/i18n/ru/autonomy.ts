// The Russian words of a project's agents working on their own: where they work, the queue, the pipeline's stages.

import type { Dictionary } from "../../i18n";

export const autonomy: Dictionary = {
  "Starts nothing by itself: switch the queue on in the project's settings": "Сама ничего не запускает: включите очередь в настройках проекта",
  "The queue looks here every half minute": "Очередь заглядывает сюда каждые полминуты",
  "{working} of {max} working": "работает {working} из {max}",
  "Starts the next card put here · {slots}": "Запустит следующую карточку, положенную сюда · {slots}",
  "Just started {task} · {slots}": "Только что запущена {task} · {slots}",
  "{task} waits: {why}": "{task} ждёт: {why}",
  "Waits: {why}": "Ждёт: {why}",
  "Pipeline done": "Конвейер пройден",
  "Pipeline stopped: {why}": "Конвейер остановлен: {why}",
  "Pipeline: the first agent's work, then {stages}": "Конвейер: работа первого агента, дальше {stages}",
  "Pipeline: the first agent's work": "Конвейер: работа первого агента",
  "Pipeline stage {n} of {total}{name}": "Этап конвейера {n} из {total}{name}",
  "1 stage": "1 этап",
  "{n} stages": "этапов: {n}",
  "Stage {n}": "Этап {n}",
  "Say where agents work first: a machine and a repository": "Сначала укажите, где работают агенты: машину и репозиторий",
  "Mark a column Queued first": "Сначала отметьте колонку как «В очереди»",
  Agents: "Агенты",
  "Where this project's agents work and which agent starts: the Start dialog begins from here, and so does the queue.":
    "Где работают агенты этого проекта и какой агент запускается: отсюда начинает и диалог запуска, и очередь.",
  Machine: "Машина",
  "Not set": "Не задано",
  Repository: "Репозиторий",
  "Pick the machine first": "Сначала выберите машину",
  "no workspace open on it now": "сейчас на нём не открыто пространство",
  Model: "Модель",
  "the agent's default": "по умолчанию у агента",
  "A saved way to start an agent": "Сохранённый способ запуска агента",
  "Start queued tasks by themselves": "Запускать задачи из очереди сами",
  "At most": "Не больше",
  "working at once": "одновременно",
  "The queue starts the top card of the Queued column once a slot is free, the machine is there, the agent's plan window is not spent and no budget over the folder is crossed — only cards you put there, and a refused one only once you move it again.":
    "Очередь запускает верхнюю карточку колонки «В очереди», когда есть свободное место, машина на связи, окно подписки агента не исчерпано и бюджет на папку не превышен, — только карточки, которые положили туда вы, а отказанную — лишь после того, как вы её снова передвинете.",
  "Add a Queued column": "Добавить колонку «В очереди»",
  Pipeline: "Конвейер",
  "Stages after the first agent's work, one after another: every prompt asks the agent to end with a line reading":
    "Этапы после работы первого агента, один за другим: каждый промпт просит агента закончить ответ строкой",
  ", and such an answer starts the next stage — its agent in a new tab of the same worktree, the card in its column. An answer without the line — a question in its own words too — or a card you move stops it; a permission prompt or a choice only waits for you. In a prompt, {task} is the task, {previous} the last stage's answer, {branch} the branch.":
    ", и такой ответ запускает следующий этап — его агента в новом табе того же worktree, карточку в его колонке. Ответ без этой строки — в том числе вопрос своими словами — или карточка, которую вы передвинули, его останавливают; запрос разрешения или выбор только ждут вас. В промпте {task} — задача, {previous} — ответ прошлого этапа, {branch} — ветка.",
  "Stage name": "Название этапа",
  model: "модель",
  "stays where it is": "остаётся на месте",
  "Remove stage": "Убрать этап",
  "Anything to add before the pull request (optional)": "Что добавить перед pull request (необязательно)",
  "What this stage's agent does: Review the work on {branch}. The last stage said: {previous}": "Что делает агент этапа: Проверь работу в {branch}. Прошлый этап сказал: {previous}",
  "then commit, push and open a pull request": "затем закоммитить, запушить и открыть pull request",
  Stage: "Этап",
  "Queued: the queue starts the tasks you put here, top first, while its switch is on": "В очереди: очередь сама запускает задачи, которые вы сюда положили, сверху вниз, пока она включена",
  Queued: "В очереди",
  "makes its tasks done. The five marks on a column say what the workflow does by itself: where the tasks agents propose land (for you to accept or decline), where your new tasks land, which column the queue starts by itself, where a task goes when an agent is started on it, where it goes when that agent finishes — unless you moved it meanwhile.":
    "делает свои задачи выполненными. Пять отметок на колонке говорят, что процесс делает сам: куда попадают задачи, которые предлагают агенты (чтобы вы приняли или отклонили), куда попадают ваши новые задачи, какую колонку очередь запускает сама, куда переходит задача, когда на ней запускают агента, и куда — когда этот агент закончил, если вы её тем временем не передвинули.",
  "The queue starts these": "Эти запускает очередь",
  "The queue could not start it: {why}. Move the card out of Queued and back to try again.": "Очередь не смогла её запустить: {why}. Уберите карточку из «В очереди» и верните, чтобы попробовать снова.",
  "The queue could not start it": "Очередь не смогла её запустить",
  "started by the queue": "запущен очередью",
  "A worktree branches off the workspace's repository, {agent} starts in it, as the project's settings say, and gets the task once it is up.":
    "От репозитория пространства ответвляется worktree, в нём запускается {agent}, как указано в настройках проекта, и получает задачу, как только поднимется.",
};
