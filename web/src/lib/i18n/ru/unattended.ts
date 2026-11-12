// The Russian words of the agents remuda starts with nobody watching: the switch that has Claude Code deny what would
// wait for a permission, where it applies, and the calls it was denied in a schedule's row and the chat's notice.

import type { Dictionary } from "../../i18n";

export const unattended: Dictionary = {
  "Don't wait for permissions: deny and carry on": "Не ждать разрешений: отказывать и продолжать",
  "Only Claude Code can deny instead of asking: {agents} would wait at its prompts.":
    "Отказывать вместо вопроса умеет только Claude Code, а не {agents}: их запросы разрешений ждут вашего ответа.",
  "Claude Code denies any call that would wait for your permission and goes on with the rest; what it was denied is listed once it has answered. In its default mode every edit asks first, so for the agent to change files add --permission-mode acceptEdits or auto to the extra arguments, or allow rules in its settings.":
    "Claude Code отказывает в любом вызове, который ждал бы вашего разрешения, и продолжает остальную работу; отклонённое перечисляется, когда он ответит. В обычном режиме каждая правка сначала спрашивает разрешения, поэтому, чтобы агент мог менять файлы, добавьте в дополнительные аргументы --permission-mode acceptEdits или auto либо правила allow в его настройки.",
  "{n} calls denied": ["{n} вызов отклонён", "{n} вызова отклонены", "{n} вызовов отклонено"],
  " · denies instead of asking": " · отказывает вместо вопроса",
  "It covers the Claude Code agents the queue starts and every stage of the pipeline; an agent you start yourself from a task asks as usual.":
    "Это касается агентов Claude Code, которых запускает очередь, и каждого этапа конвейера; агент, которого вы запускаете из задачи сами, спрашивает как обычно.",
  "A review only reads: whatever Claude Code would ask your permission for is denied, and it goes on to its answer instead of waiting in its tab.":
    "Ревью только читает: во всём, на что Claude Code спросил бы вашего разрешения, будет отказано, и он перейдёт к ответу, а не будет ждать в своём табе.",
  "Open in the terminal": "Открыть в терминале",
  "Claude Code was denied {n} calls nobody was there to approve:": [
    "Claude Code отказано в {n} вызове, который некому было одобрить:",
    "Claude Code отказано в {n} вызовах, которые некому было одобрить:",
    "Claude Code отказано в {n} вызовах, которые некому было одобрить:",
  ],
};
