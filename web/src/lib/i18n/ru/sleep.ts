// The Russian words of a workspace put to sleep and woken, "Resume after restart" and the two switches beside them.

import type { Dictionary } from "../../i18n";

export const sleep: Dictionary = {
  "{agent} is waiting for your answer": "{agent} ждёт вашего ответа",
  "{agent} is working": "{agent} работает",
  "Sleep {name}? Still busy: {busy}. Sleeping interrupts them. Waking resumes their conversations where they stopped.":
    "Усыпить {name}? Ещё заняты: {busy}. Сон прервёт их. Пробуждение продолжит их разговоры с того места, где они остановились.",
  "{name} is asleep · {n} agents stopped. Wake it from its menu.": [
    "{name} спит · {n} агент остановлен. Разбудите его из меню.",
    "{name} спит · {n} агента остановлены. Разбудите его из меню.",
    "{name} спит · {n} агентов остановлены. Разбудите его из меню.",
  ],
  "{name} is awake · {n} agents resumed": ["{name} проснулось · {n} агент продолжил работу", "{name} проснулось · {n} агента продолжили работу", "{name} проснулось · {n} агентов продолжили работу"],
  "Asleep · {n} agents stopped · Wake from the menu": ["Спит · {n} агент остановлен · разбудить из меню", "Спит · {n} агента остановлены · разбудить из меню", "Спит · {n} агентов остановлены · разбудить из меню"],
  "Resume {n}": "Возобновить {n}",
  "Sleep anyway": "Всё равно усыпить",
  "Putting {name} to sleep…": "Усыпляю {name}…",
  "{name} did not go to sleep": "{name} не уснуло",
  "Waking {name}…": "Бужу {name}…",
  "{name} is still partly asleep": "{name} ещё отчасти спит",
  Wake: "Разбудить",
  "{n} agents back on their conversations": ["{n} агент вернётся к своему разговору", "{n} агента вернутся к своим разговорам", "{n} агентов вернутся к своим разговорам"],
  "Sleep…": "Усыпить…",
  "its agents leave, their shells stay; waking resumes them": "агенты выходят, их оболочки остаются; пробуждение продолжит их работу",
  Asleep: "Спит",
  "Sleep {workspace}…": "Усыпить {workspace}…",
  "Wake {workspace}": "Разбудить {workspace}",
  "asleep · {n} agents back on their conversations": ["спит · {n} агент вернётся к своему разговору", "спит · {n} агента вернутся к своим разговорам", "спит · {n} агентов вернутся к своим разговорам"],
  "Resume after restart on {machine}…": "Возобновить после перезапуска на {machine}…",
  "Resume after restart": "Возобновить после перезапуска",
  "These were running before their panes came back as bare shells — a reboot, or herdr's server started again. Each comes back in its own pane, on its own conversation.":
    "Они работали, пока их панели не вернулись пустыми оболочками — после перезагрузки или нового запуска сервера herdr. Каждый вернётся в своей панели, к своему разговору.",
  "They are being resumed right now.": "Они возобновляются прямо сейчас.",
  "Nothing waits on {machine}: every chat stopped there is back, or was left as it was.": "На {machine} ничего не ждёт: каждый остановленный там чат вернулся или оставлен как есть.",
  "Chats stopped with the reboot": "Чаты, остановленные перезагрузкой",
  "Each agent comes back with its own resume command; a model or arguments it was started with are not carried over, since herdr does not keep them.":
    "Каждый агент вернётся своей командой возобновления; модель и аргументы, с которыми его запускали, не переносятся: herdr их не хранит.",
  "Resume automatically next time": "В следующий раз возобновлять автоматически",
  "Not now": "Не сейчас",
  "Resume all": "Возобновить все",
  "Resume…": "Возобновить…",
  "Sleep and resume": "Сон и возобновление",
  "Wake a sleeping workspace when I open it": "Будить спящее пространство, когда я его открываю",
  "A workspace put to sleep from its menu wakes the moment you open one of its panes, its agents back on their conversations. Off, Wake in its menu does it.":
    "Пространство, усыплённое из его меню, просыпается, как только вы открываете одну из его панелей, и агенты возвращаются к своим разговорам. Если выключено, это делает «Разбудить» в его меню.",
  "When a machine comes back from a reboot, the chats it stopped start again on their sessions by themselves, a row in the activity saying what came back. Off, a toast and the machine's ⓘ offer them in Resume after restart.":
    "Когда машина возвращается после перезагрузки, остановленные чаты сами запускаются снова на своих сессиях, а строка в событиях говорит, что вернулось. Если выключено, их предлагают уведомление и ⓘ машины в окне «Возобновить после перезапуска».",
};
