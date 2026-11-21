// The Russian words of Fleet setup: the table of how every machine is set up, its rows, values and marks, the updates
// that bring a machine in line and what is left to do by hand.

import type { Dictionary } from "../../i18n";

export const fleet: Dictionary = {
  "The remuda-core on each machine: the terminal runtime remuda's agents run in.": "remuda-core на каждой машине: среда терминалов, в которой работают агенты remuda.",
  "Install {agent} on {machines} with its own installer — or leave it, if it is not wanted there.": "Установите {agent} на {machines} его собственным установщиком — или оставьте как есть, если он там не нужен.",
  "Fleet setup": "Настройка машин",
  "Fleet setup…": "Настройка машин…",
  "how every machine is set up, side by side, the odd one out marked": "как настроена каждая машина, рядом друг с другом, с отметкой у выбивающейся",
  "every machine side by side": "все машины рядом",
  "Reading every machine…": "Читаю каждую машину…",
  "Show every fact": "Показать всё",
  "Read again": "Прочитать снова",
  "Every machine side by side, read {when}: the value most of them share is the norm, and one that differs is marked.":
    "Все машины рядом, прочитаны {when}: норма — значение, общее для большинства, а отличающееся отмечено.",
  "Bring in line": "Привести к норме",
  "Run…": "Запустить…",
  "The command": "Команда",
  "By hand": "Вручную",
  "On one machine only, so nothing to compare: {list}.": "Только на одной машине, сравнивать не с чем: {list}.",
  Fact: "Что",
  away: "не на связи",
  "not read": "не прочитана",
  "No value most machines share": "Нет значения, общего для большинства машин",
  "Most machines: {value}": "У большинства машин: {value}",

  "Claude Code's settings": "Настройки Claude Code",
  "Global instructions": "Общие инструкции",
  "{agent}: installed with": "{agent}: установлен через",
  "Tasks for {agent} (MCP)": "Задачи для {agent} (MCP)",
  "Plan limits in the status line": "Лимиты плана в строке статуса",
  "Default model": "Модель по умолчанию",
  "Fast mode": "Быстрый режим",
  "Behind its upstream": "Отстаёт от upstream",

  "The {agent} version on each machine, as the vitals last read it.": "Версия {agent} на каждой машине, как её в последний раз прочитала проверка состояния.",
  "How {agent} was installed there — Homebrew, npm or its own installer — which decides how it is updated.":
    "Как там установлен {agent} — через Homebrew, npm или собственный установщик: от этого зависит, как он обновляется.",
  "The remuda on each machine: it keeps the agents' door to your tasks there.":
    "remuda на каждой машине: она держит там дверь агентов к вашим задачам.",
  "The code-server the Code mode opens on each machine.": "code-server, который режим «Код» открывает на каждой машине.",
  "Whether remuda's MCP server — your task list for the agents — is registered in {agent}'s own config there.":
    "Зарегистрирован ли MCP-сервер remuda — ваш список задач для агентов — в собственной конфигурации {agent} там.",
  "Whether Claude Code's status line goes through remuda there, which keeps its plan limits for the meter.":
    "Идёт ли строка статуса Claude Code там через remuda, которая сохраняет лимиты плана для счётчика.",
  "The default model Claude Code's user settings keep there, in ~/.claude/settings.json.": "Модель по умолчанию в пользовательских настройках Claude Code там, в ~/.claude/settings.json.",
  "Whether fast mode is saved on in Claude Code's user settings there.": "Сохранён ли быстрый режим включённым в пользовательских настройках Claude Code там.",
  "{file}: what {agent} reads in every session there, compared by a hash of its content.": "{file}: то, что {agent} читает там в каждой сессии; сравнивается по хешу содержимого.",
  "The branch checked out in the repository's main checkout on each machine.": "Ветка, на которой стоит основная копия репозитория на каждой машине.",
  "Commits on the checkout's upstream it does not have yet, as of the last fetch there.": "Коммиты upstream, которых в копии ещё нет, на момент последнего fetch там.",
  "{file} in the repository's main checkout, compared by a hash of its content.": "{file} в основной копии репозитория; сравнивается по хешу содержимого.",

  "not installed": "не установлен",
  "another way": "иначе",
  on: "вкл.",
  yes: "да",
  no: "нет",
  current: "актуальны",
  outdated: "устарели",
  missing: "нет",
  local: "локальный",
  "Claude Code's default": "по умолчанию Claude Code",
  absent: "нет файла",
  present: "есть",
  "up to date": "актуальна",
  "{n} commits behind": ["отстаёт на {n} коммит", "отстаёт на {n} коммита", "отстаёт на {n} коммитов"],
  "sha256 {hash}, changed {when}": "sha256 {hash}, изменён {when}",
  "against {upstream}, fetched {when}": "относительно {upstream}, fetch {when}",
  "against {upstream}, never fetched there": "относительно {upstream}, fetch там ещё не делали",
  "Connect a second machine to compare how they are set up.": "Подключите вторую машину, чтобы сравнить, как они настроены.",
  "Every machine is set up alike: {n} facts compared.": [
    "Все машины настроены одинаково: сравнён {n} параметр.",
    "Все машины настроены одинаково: сравнено {n} параметра.",
    "Все машины настроены одинаково: сравнено {n} параметров.",
  ],
  "{n} of {total} facts differ between the machines.": [
    "Между машинами отличается {n} параметр из {total}.",
    "Между машинами отличаются {n} параметра из {total}.",
    "Между машинами отличаются {n} параметров из {total}.",
  ],
  "the copy changed last is {machine}'s, {when}": "последней менялась копия на {machine}, {when}",

  "Update {agent} on {machines} by hand: remuda has no update for it there.": "Обновите {agent} на {machines} вручную: у remuda там нет для него обновления.",
  "{agent} is installed another way on {machines}; remuda updates each the way it was installed. To match, reinstall it there by hand.":
    "На {machines} {agent} установлен иначе; remuda обновляет каждый так, как он был установлен. Чтобы совпадало, переустановите его там вручную.",
  "Install the same remuda on {machines}: the installer's one line there, or Updates once a newer release is out.":
    "Установите ту же remuda на {machines}: одной строкой установщика там или через «Обновления», когда выйдет новый релиз.",
  "Run remuda code install on {machines}.": "Запустите remuda code install на {machines}.",
  "Run remuda core install on {machines}.": "Запустите remuda core install на {machines}.",
  "Run remuda mcp install on {machines}.": "Запустите remuda mcp install на {machines}.",
  "Run remuda statusline install on {machines}, or remuda statusline uninstall where it is not wanted.":
    "Запустите remuda statusline install на {machines} или remuda statusline uninstall там, где она не нужна.",
  "Set Claude Code's default model on {machines} by hand: /model in Claude Code there saves the pick, or edit ~/.claude/settings.json. remuda never writes the agents' settings.":
    "Задайте модель по умолчанию Claude Code на {machines} вручную: /model в Claude Code там сохранит выбор, или поправьте ~/.claude/settings.json. remuda никогда не пишет в настройки агентов.",
  "Fast mode is set otherwise on {machines}: switch it in Claude Code there, or in ~/.claude/settings.json. remuda never writes the agents' settings.":
    "На {machines} быстрый режим настроен иначе: переключите его в Claude Code там или в ~/.claude/settings.json. remuda никогда не пишет в настройки агентов.",
  "{file} differs on {machines}{tail}: copy the one you want to the others by hand. remuda never writes the agents' configs.":
    "{file} отличается на {machines}{tail}: скопируйте нужный на остальные вручную. remuda никогда не пишет в конфигурацию агентов.",
  "Another branch is checked out on {machines}: switch it there, unless that is on purpose.": "На {machines} выбрана другая ветка: переключите её там, если это не намеренно.",
  "Pull on {machines}: the checkout there is behind its upstream as of its last fetch.": "Сделайте pull на {machines}: копия там отстаёт от upstream на момент последнего fetch.",
  "{file} differs on {machines}{tail}: commit it where it changed and pull it on the others.": "{file} отличается на {machines}{tail}: закоммитьте его там, где он менялся, и сделайте pull на остальных.",
  "Differs on {machines}.": "Отличается на {machines}.",
};
