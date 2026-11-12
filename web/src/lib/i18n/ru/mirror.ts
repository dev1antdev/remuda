// The Russian words of Inside herdr: remuda's figures as herdr's tokens, its alerts as herdr's toasts, a test toast.

import type { Dictionary } from "../../i18n";

export const mirror: Dictionary = {
  "Inside herdr": "Внутри herdr",
  "This remuda is a scratch instance beside the real one: it writes nothing into herdr, so the two never fight over the same panes.":
    "Этот remuda — пробный экземпляр рядом с настоящим: он ничего не пишет в herdr, чтобы двое не спорили за одни и те же панели.",
  "Show remuda's figures in herdr": "Показывать цифры remuda в herdr",
  "On by default. Each agent's cost, context, model, pull request, task, the watchdog's note and its last answer go to herdr as tokens on its pane, a workspace's review and cost on the workspace — written when they move, gone within five minutes once remuda stops. Off clears them.":
    "Включено по умолчанию. Стоимость каждого агента, контекст, модель, пул-реквест, задача, заметка сторожа и его последний ответ уходят в herdr токенами на его панель, ревью и стоимость рабочего пространства — на само пространство; пишутся, когда меняются, и исчезают в течение пяти минут после остановки remuda. Выключение стирает их.",
  "herdr shows a token only where a sidebar row of its config.toml names it, and a row whose tokens have no value disappears. Put these rows there — in place of the rows of a [ui.sidebar.agents] or [ui.sidebar.spaces] it already has, since a table written twice makes herdr ignore the whole file — check it with herdr config check, then run herdr server reload-config:":
    "herdr показывает токен только там, где его называет строка боковой панели в config.toml, а строка, у токенов которой нет значений, исчезает. Поставьте туда эти строки — вместо строк уже имеющихся [ui.sidebar.agents] или [ui.sidebar.spaces]: таблица, записанная дважды, заставляет herdr пропустить весь файл, — проверьте его командой herdr config check и выполните herdr server reload-config:",
  "Set delivery under the [ui.toast] its config.toml may already have — a second one makes herdr ignore the whole file — or add the table where there is none:":
    "Задайте delivery в таблице [ui.toast], если она уже есть в config.toml, — вторая такая таблица заставляет herdr пропустить весь файл, — или добавьте таблицу, если её нет:",
  "The watchdog's note also stands in state_text, in place of the state it is about.": "Заметка сторожа также встаёт в state_text — вместо состояния, к которому она относится.",
  "Alerts inside herdr too": "Оповещения и внутри herdr",
  "Off by default. A question, a finish after a turn over a minute and the watchdog's notes also pop up as herdr's own toast on the machine the agent runs on — never over the tab herdr has in front, never for a muted workspace. One switch for every machine: herdr shows a toast only where its config.toml turns its toasts on, which it does not by default.":
    "Выключено по умолчанию. Вопрос, завершение хода дольше минуты и заметки сторожа также всплывают собственным тостом herdr на машине, где работает агент, — никогда поверх вкладки, открытой в herdr, и никогда для заглушённого рабочего пространства. Один переключатель на все машины: herdr показывает тост только там, где его config.toml включает тосты, а по умолчанию они выключены.",
  "No toast pops up between the two times, by the clock of {zone}.": "Между этими двумя временами тосты не всплывают, по часам {zone}.",
  "Send a test toast": "Отправить тестовый тост",
  "to every connected machine, whatever the switches say": "на каждую подключённую машину, что бы ни говорили переключатели",
  "Copied for herdr's config.toml": "Скопировано для config.toml herdr",
  "It could not be copied; select the text instead": "Скопировать не удалось; выделите текст вручную",
  "{machine}: herdr was not reached — {error}": "{machine}: до herdr не достучаться — {error}",
  "{machine}: shown": "{machine}: показано",
  "{machine}: herdr's toasts are off there; set ui.toast.delivery in its config.toml": "{machine}: тосты herdr там выключены; задайте ui.toast.delivery в его config.toml",
  "{machine}: nobody has herdr open there": "{machine}: там herdr никто не открыл",
  "{machine}: herdr held it back, too many toasts at once": "{machine}: herdr придержал его — слишком много тостов сразу",
  "{machine}: another toast was still up there": "{machine}: там ещё висел другой тост",
  "{machine}: not shown ({reason})": "{machine}: не показано ({reason})",
  "On {machine} the remuda at {writer} writes into herdr and pops its toasts, so this one stays out there.":
    "На {machine} в herdr пишет и показывает тосты remuda на {writer}, поэтому этот туда не вмешивается.",
  "On {machine} another remuda writes into herdr and pops its toasts, so this one stays out there.":
    "На {machine} в herdr пишет и показывает тосты другой remuda, поэтому этот туда не вмешивается.",
};
