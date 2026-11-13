// The Russian words of the chat's prompts list, its live progress line, the messages sent while an agent worked and the
// tool calls' cards.

import type { Dictionary } from "../../i18n";

export const chatprompts: Dictionary = {
  Prompts: "Промпты",
  "Prompts list": "Список промптов",
  "Prompts: the conversation prompt by prompt, newest first": "Промпты: разговор промпт за промптом, новые сверху",
  "Close the prompts list": "Закрыть список промптов",
  "Show the prompts list": "Показать список промптов",
  "Hide the prompts list": "Скрыть список промптов",
  "prompt by prompt, newest first: what each took, wrote and changed": "промпт за промптом, новые сверху: сколько занял каждый, что написал и изменил",
  "No prompts yet.": "Промптов пока нет.",
  "No reply text.": "Ответа текстом нет.",
  Changed: "Изменено",
  "context {n}": "контекст {n}",
  "done {time}": "готово {time}",
  "↓ {n} tokens": "↓ {n} токенов",
  "{n} commands run": ["{n} команда", "{n} команды", "{n} команд"],
  "+ {n} more messages: {text}": ["+ ещё {n} сообщение: {text}", "+ ещё {n} сообщения: {text}", "+ ещё {n} сообщений: {text}"],
  "progress|Working": "Работает",
  "Sent while it worked": "Отправлено, пока агент работал",
  "From {agent}, while it worked": "От {agent}, пока агент работал",
  "You, while it worked": "Вы, пока агент работал",
  "{agent}, while it worked": "{agent}, пока агент работал",
  Failed: "Не удалось",
  Running: "Выполняется",
  "No result": "Без результата",
  "from your prompt to the end of this answer; ↓ the tokens it wrote, as the CLI counts them; the tokens of its API responses, cache included, at API prices — on a subscription a measure, not a bill":
    "от вашего промпта до конца этого ответа; ↓ — токены, которые агент написал, как их считает CLI; токены его ответов API вместе с кешем, по ценам API — при подписке это мера, а не счёт",
};
