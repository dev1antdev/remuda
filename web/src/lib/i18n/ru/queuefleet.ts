// The Russian words of the plan limits seen coming and the queue that runs on any machine with room: the forecast of
// each window, the offers to run held work on the other vendor's agent, the machine a fleet queue picks and why, the
// battery in the vitals.

import type { Dictionary } from "../../i18n";

export const queuefleet: Dictionary = {
  "runs out ≈ {time} at this pace": "закончится ≈ в {time} при таком темпе",
  "lasts to its reset at this pace": "при таком темпе хватит до сброса",
  "Task {ref} “{title}”": "Задача {ref} «{title}»",
  "Schedule “{title}”": "Расписание «{title}»",
  "{what} waits for {agent}'s plan limit.": "{what} ждёт: лимит плана {agent} на исходе.",
  "Run on {agent}": "Запустить на {agent}",
  "{agent} {window} runs out ≈ {at} at this pace, before its reset at {time}": "{agent} {window}: при таком темпе закончится ≈ в {at}, раньше сброса в {time}",
  "{agent} {window} is spent, resets {time}": "{agent} {window}: исчерпано, сброс {time}",
  "{agent} {window} is at {used} %, resets {time}": "{agent} {window}: {used} %, сброс {time}",
  "on {model}, as your recipe says": "на {model}, как в вашем рецепте",
  "on its default model": "на его модели по умолчанию",
  "{what} started on {agent}.": "{what}: запущено на {agent}.",
  "{what} did not start on {agent}: {why}": "{what}: не запустилось на {agent}: {why}",
  "Let it wait": "Пусть ждёт",
  Battery: "Батарея",
  "on battery": "от батареи",
  charging: "заряжается",
  "on power": "от сети",
  "load {load} per core": "нагрузка {load} на ядро",
  "warm at {temp} °C": "тёплый, {temp} °C",
  "hot at {temp} °C": "горячий, {temp} °C",
  "on battery at {pct} %": "от батареи, {pct} %",
  "no agent working": "агенты не работают",
  "{n} agents working": ["работает {n} агент", "работают {n} агента", "работают {n} агентов"],
  "{agent} at {pct} %": "{agent} на {pct} %",
  "next on {machine}: {facts}": "следующая — на {machine}: {facts}",
  "no connected machine has a workspace open on a checkout of this repository": "ни на одной машине на связи не открыто пространство на копии этого репозитория",
  "no machine has room: {why}": "ни на одной машине нет места: {why}",
  "Starts the next card put here on the machine with room · {slots}": "Запускает следующую карточку, положенную сюда, на машине, где есть место · {slots}",
  "Just started {task} on {machine} · {slots}": "Только что запущена {task} на {machine} · {slots}",
  "The queue starts agents on": "Очередь запускает агентов на",
  "This machine only": "Только этой машине",
  "Any machine that has room": "Любой машине, где есть место",
  "A queued card starts on the connected machine with the most room that has a workspace open on a checkout of this repository, matched by its remote: the lightest load per core, a cool CPU, on its charger, room on the agent's plan window, the fewest agents working. A hot machine, one on a nearly empty battery, one at two of load per core or over its budget is passed over; this remuda's own host never runs one when it only serves the page.":
    "Карточка из очереди запускается на машине на связи, где больше всего места и открыто пространство на копии этого репозитория (сверяется по remote): наименьшая нагрузка на ядро, прохладный процессор, питание от сети, запас в окне подписки агента, меньше всего работающих агентов. Горячая машина, машина на почти разряженной батарее, с нагрузкой от двух на ядро или с превышенным бюджетом пропускается; хост этого remuda, который только отдаёт страницу, агентов не запускает никогда.",
  "skipped: its agent's plan window is nearly spent": "пропущен: окно подписки его агента на исходе",
};
