// The Russian words of the chat's quote of an answer and the queue of notes for the agent.

import type { Dictionary } from "../../i18n";

export const quote: Dictionary = {
  Quote: "Цитировать",
  "Quote it in your reply (Q)": "Процитировать в ответе (Q)",
  "quote the text selected in an answer into your reply": "процитировать выделенный кусок ответа в своём ответе",
  "keep the text as a note in the agent's queue, sent only when you press Send on it": "оставить текст заметкой в очереди агента — уйдёт, только когда нажмёте у неё «Отправить»",
  "Queue it as a note for the agent (Alt+↵): sent only when you press Send on it": "Отложить заметкой для агента (Alt+↵): уйдёт, только когда нажмёте у неё «Отправить»",
  "Add to the queue": "В очередь",
  "Notes queued for the agent": "Заметки в очереди для агента",
  "{n} notes queued for the agent — nothing goes until you send them": [
    "{n} заметка в очереди для агента — ничего не уйдёт, пока вы её не отправите",
    "{n} заметки в очереди для агента — ничего не уйдёт, пока вы их не отправите",
    "{n} заметок в очереди для агента — ничего не уйдёт, пока вы их не отправите",
  ],
  "Send all as one message": "Отправить все одним сообщением",
  "Click to put it back in the box": "Нажмите, чтобы вернуть в поле ввода",
  "Drop this note": "Убрать заметку",
};
