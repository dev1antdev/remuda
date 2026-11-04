// The Russian words the 0.12.0 audit of the panels and dialogs added: words that had none, and better ones.

import type { Dictionary } from "../../i18n";

export const auditpanels: Dictionary = {
  "Claude Code shows its plan limits to its status line alone, and remuda reads them there once you let it. Not set up yet on {machines}: the machine's ⓘ → Updates → “Show Claude Code's plan limits”.": "Claude Code показывает лимиты плана только своей строке состояния, и remuda читает их там, если это разрешить. Пока не включено на {machines}: ⓘ машины → Обновления → «Show Claude Code's plan limits».",
  "no origin remote, so no owner's avatar": "у репозитория нет origin, поэтому нет аватара владельца",
  "the origin remote names no host and owner": "в адресе origin нет хоста и владельца",
  "GitHub has no avatar for {name}": "у GitHub нет аватара для {name}",
  "GitLab has no avatar for {name} or its group": "у GitLab нет аватара для {name} и его группы",
  "no glab on {name} to read the GitLab avatar": "на {name} нет glab, чтобы прочитать аватар GitLab",
  "glab could not be asked on {name}": "не удалось спросить glab на {name}",
  ready: "готов",
  shell: "терминал",
  "New tab…": "Новая вкладка…",
  "New worktree…": "Новый worktree…",
  "Rename workspace…": "Переименовать пространство…",
  Planner: "Планировщик",
  Today: "Сегодня",
  Yesterday: "Вчера",
  "Keeping…": "Оставляю…",
  "When an agent": "Когда агент",
  finishes: "заканчивает",
  "Turns ≥ 1 min, a minute after; “always” means every turn, after 10 s.": "Ходы от 1 мин — через минуту; «всегда» — каждый ход, через 10 с.",
  never: "никогда",
  always: "всегда",
  "Offered under": "Предлагаются под",
  and: "и",
  "in the composer; one tap puts the text in. Shared by every device that uses this remuda.":
    "в поле ввода; одно нажатие вставляет текст. Общие для всех устройств этого remuda.",
  "become the pane's folder, its branch and the file you last opened.": "превращаются в папку панели, её ветку и файл, открытый последним.",
  "only {folder}": "только {folder}",
  forward: "переслать",
  "code-server is not running on {machine}": "code-server не запущен на {machine}",
  "Install it there as a background service (VS Code in the browser, bound to 127.0.0.1:{port} with no auth of its own — remuda's token is the door):":
    "Установите его там как фоновую службу (VS Code в браузере, на 127.0.0.1:{port} без своей авторизации — вход через токен remuda):",
  "…in a terminal on the machine that serves this page (": "…в терминале машины, которая отдаёт эту страницу (",
  " when it is not on your PATH). It downloads a pinned release of code-server and verifies its checksum.":
    ", если его нет в PATH). Скачивается закреплённый релиз code-server, его контрольная сумма проверяется.",
  reviewed: "просмотрен",
  repo: "репо",
  Options: "Параметры",
  "Machines saved in herdr appear on their own. This adds another: any host your":
    "Машины, сохранённые в herdr, появляются сами. Здесь добавляется ещё одна: любой хост, до которого ваш",
  "reaches with keys, with herdr running there.": "достаёт по ключам и где запущен herdr.",
  "A prompt sent to the agent running in a folder, the standup pushed to your devices as a digest, or a fresh agent started in a folder with a prompt, on a timetable — in the remuda host's local time. A minute with no agent there, or an agent busy with a question, is skipped and noted.":
    "Промпт агенту, работающему в папке, стендап дайджестом на ваши устройства или новый агент в папке с промптом — по расписанию, по местному времени хоста remuda. Минута, когда агента там нет или он занят вопросом, пропускается и отмечается.",
  "last {ago}: {result}": "последний запуск {ago}: {result}",
  you: "вы",
  "The issue is read with": "Issue читается через",
  "on this machine": "на этой машине",
  ", as you; its title, text and labels become a task that names it, so an agent started on the task knows where it came from. Nothing is written to GitHub.":
    " от вашего имени; его заголовок, текст и метки становятся задачей со ссылкой на него, так что агент, запущенный на задаче, знает, откуда она. В GitHub ничего не пишется.",
  "create “{name}”": "создать «{name}»",
  "A task is always in one column, and a column that is": "Задача всегда стоит в одной колонке, а колонка вида",
  "makes its tasks done. The four marks on a column say what the workflow does by itself: where the tasks agents propose land (for you to accept or decline), where your new tasks land, where a task goes when an agent is started on it, where it goes when that agent finishes — unless you moved it meanwhile.":
    "делает свои задачи выполненными. Четыре метки колонки говорят, что процесс делает сам: куда попадают задачи, предложенные агентами (чтобы вы их приняли или отклонили), куда попадают ваши новые задачи, куда задача переходит, когда на ней запускают агента, и куда — когда этот агент заканчивает, если вы её тем временем не передвинули.",
  "What this project's tasks carry besides a title, description, due date, priority and labels. A field":
    "Что несут задачи этого проекта помимо заголовка, описания, срока, приоритета и меток. Поле",
  "shows beside the title; one": "показывается рядом с заголовком; поле",
  "A worktree branches off the workspace's repository, Claude starts in it and gets the task once it is up.":
    "От репозитория пространства ответвляется worktree, в нём запускается Claude и получает задачу, как только поднимется.",
  "The task moves to": "Задача переходит в",
  "The task stays where it is;": "Задача остаётся на месте;",
  "Running on {machine}… (closing this keeps it running)": "Выполняется на {machine}… (если закрыть окно, работа продолжится)",
  "That turn is older than the loaded part of this transcript (the last 16 MB).": "Этот ход старше загруженной части записи (последние 16 МБ).",
  "No textual diff (binary, or identical).": "Текстовых изменений нет (двоичный файл или без отличий).",
  "Name (optional)": "Название (необязательно)",
  "Workspace name (optional)": "Название пространства (необязательно)",
  "Label (optional)": "Имя (необязательно)",
  "(optional)": "(необязательно)",
  "Each run opens a fresh tab in the folder's workspace (or a workspace on the folder) and hands the agent the prompt once it is up. A question the agent asks is yours to answer: nothing closes while it waits. The conversation stays readable through the search after the tab is gone.":
    "Каждый запуск открывает новую вкладку в пространстве папки (или пространство на папке) и отдаёт агенту промпт, как только он поднимется. На вопрос агента отвечаете вы: пока он ждёт, ничего не закрывается. Разговор остаётся доступным через поиск и после того, как вкладки не станет.",
  "What the agents on each machine use to read and write your tasks through remuda mcp and remuda tasks, and to read who is doing what (the roster, a recap, the standup): never the rest. A revoked one is replaced when remuda next connects to that machine (this one at its next start).":
    "Ими агенты на каждой машине читают и пишут ваши задачи через remuda mcp и remuda tasks и смотрят, кто чем занят (список, сводка, стендап) — и ничего больше. Отозванный ключ заменяется, когда remuda в следующий раз подключится к той машине (к этой — при следующем запуске).",
  "Every key the page answers (also behind ?)": "Все клавиши, на которые отвечает страница (ещё за ?)",
  "A note for the agent (optional)": "Заметка для агента (необязательно)",
  "Not done": "Не готово",
  to: "до",
  "{n} commits on no remote": ["{n} коммит не на удалённом", "{n} коммита не на удалённом", "{n} коммитов не на удалённом"],
  "{n} staged files": ["{n} файл в индексе", "{n} файла в индексе", "{n} файлов в индексе"],
  "{n} changed files": ["{n} изменённый файл", "{n} изменённых файла", "{n} изменённых файлов"],
  "{n} new files": ["{n} новый файл", "{n} новых файла", "{n} новых файлов"],
};
