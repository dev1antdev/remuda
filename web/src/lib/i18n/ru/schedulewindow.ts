// The Russian words of a schedule every few minutes: the preset and its window, a cron in words, the next runs, and a
// run skipped while the agent it would reach is still at it.

import type { Dictionary } from "../../i18n";

export const schedulewindow: Dictionary = {
  "Every N minutes": "Каждые N минут",
  Every: "Каждые",
  "{n} min": "{n} мин",
  Until: "До",
  "Weekdays only": "Только по будням",
  "Mon–Fri": "Пн–Пт",
  weekends: "выходные",
  "every day": "каждый день",
  "on day {days} of the month": "{days} числа",
  " or ": " или ",
  "in {months}": "в {months}",
  "every hour at :{mm}": "каждый час в :{mm}",
  "at {time}": "в {time}",
  "every {n} h at :{mm}": "каждые {n} ч в :{mm}",
  "every {n} h from {time}": "каждые {n} ч с {time}",
  "at {times}": "в {times}",
  "every minute": "каждую минуту",
  "every {n} min": "каждые {n} мин",
  "every {n} min from :{mm}": "каждые {n} мин с :{mm}",
  "at minutes {list}": "в минуты {list}",
  "in the hours {hours}": "в часы {hours}",
  "on the host's clock, UTC{offset}": "по часам хоста, UTC{offset}",
  "next: {times}": "дальше: {times}",
  "No run within a year": "За год ни одного запуска",
  "skipped: the agent is still working": "пропущен: агент ещё работает",
  "skipped: the agent waits for an answer": "пропущен: агент ждёт ответа",
  "skipped: the agent has not started on the prompt yet": "пропущен: агент ещё не взялся за промпт",
  "skipped: no word yet from its machine": "пропущен: машина ещё не сообщила, как дела у агента",
  "skipped: the previous run is still on its way": "пропущен: прошлый запуск ещё идёт",
  "A prompt sent to the agent running in a folder, the standup pushed to your devices as a digest, a fresh agent started in a folder with a prompt, or one of your scenes, on a timetable — in the remuda host's local time. A run is skipped and noted while the agent it would reach — or the one its last run started — is still working or waits for an answer, so nothing piles up; a minute with no agent there is noted and pushed.":
    "Промпт агенту, работающему в папке, стендап дайджестом на ваши устройства, новый агент в папке с промптом или одна из ваших сцен — по расписанию, по местному времени хоста remuda. Пока агент, к которому идёт запуск (или тот, кого запустил прошлый), ещё работает или ждёт ответа, запуск пропускается и отмечается, так что ничего не копится; минута, когда агента там нет, отмечается и присылается уведомлением.",
  "Each run opens a fresh tab in the folder's workspace (or a workspace on the folder) and hands the agent the prompt once it is up. While the agent the last run started still works or asks something, the next run is skipped. A question the agent asks is yours to answer: nothing closes while it waits. The conversation stays readable through the search after the tab is gone.":
    "Каждый запуск открывает новую вкладку в пространстве папки (или пространство на папке) и отдаёт агенту промпт, как только он поднимется. Пока агент прошлого запуска ещё работает или о чём-то спрашивает, следующий запуск пропускается. На вопрос агента отвечаете вы: пока он ждёт, ничего не закрывается. Разговор остаётся доступным через поиск и после того, как вкладки не станет.",
};
