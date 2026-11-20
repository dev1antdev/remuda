// The Russian words of the chats a reboot stopped and of herdr's agents carried on in remuda-core: the ⓘ concern, the
// toast, the Resume button and what follows it, herdr's server stopped.

import type { Dictionary } from "../../i18n";

export const recover: Dictionary = {
  "{n} chats stopped with the reboot": ["{n} чат остановлен перезагрузкой", "{n} чата остановлены перезагрузкой", "{n} чатов остановлены перезагрузкой"],
  "{agent} in {title}": "{agent} в {title}",
  "Resume": "Возобновить",
  "Resuming {n} chats…": ["Возобновляю {n} чат…", "Возобновляю {n} чата…", "Возобновляю {n} чатов…"],
  "The chats are back on their sessions": ["Чат вернулся к своей сессии", "Чаты вернулись к своим сессиям", "Чаты вернулись к своим сессиям"],
  "Not every chat came back": "Вернулись не все чаты",
  "{n} agents from herdr on {machine} can carry on": [
    "{n} агент из herdr на {machine} может продолжить работу",
    "{n} агента из herdr на {machine} могут продолжить работу",
    "{n} агентов из herdr на {machine} могут продолжить работу",
  ],
  "from herdr": "из herdr",
  "The ones from herdr ran there before the machine moved to remuda-core: each comes back in a tab of a workspace on its folder.":
    "Агенты из herdr работали там до перехода машины на remuda-core: каждый вернётся во вкладке пространства на своей папке.",
  "herdr's server still runs on {machine}, and its agents may still be on these conversations: stop it first, or each would run twice.":
    "Сервер herdr на {machine} ещё работает, и его агенты, возможно, ещё ведут эти разговоры: сначала остановите его, иначе каждый запустится дважды.",
  "Stop herdr's server on {machine}…": "Остановить сервер herdr на {machine}…",
  "Stop herdr's server on {machine}? All agents still running in herdr there stop; herdr stays installed.":
    "Остановить сервер herdr на {machine}? Все агенты, ещё работающие там в herdr, остановятся; herdr останется установлен.",
  "Stop herdr's server": "Остановить сервер herdr",
  "herdr's server on {machine} is stopped": "Сервер herdr на {machine} остановлен",
};
