// The Russian words of the project's files: the files rail's two tabs, the tree and its row menu, the file handed to an
// agent, and the file viewer's changed lines.

import type { Dictionary } from "../../i18n";

export const filestree: Dictionary = {
  Project: "Проект",
  "More: {name}": "Ещё: {name}",
  "Show changes": "Показать изменения",
  "Copy relative path": "Копировать относительный путь",
  "Copy full path": "Копировать полный путь",
  "Send to agent…": "Отправить агенту…",
  "Copied {path}": "Скопировано: {path}",
  "The path could not be copied": "Путь не удалось скопировать",
  "Send a file to an agent": "Отправить файл агенту",
  "Send a folder to an agent": "Отправить папку агенту",
  "What should it do?": "Что ему сделать?",
  "Sent to {agent} in {workspace}: {name}": "Отправлено агенту {agent} в {workspace}: {name}",
  "Added, staged for the next commit": "Добавлен, подготовлен к следующему коммиту",
  "Changed since HEAD": "Изменён после HEAD",
  "Deleted since HEAD": "Удалён после HEAD",
  "In conflict: a merge stopped on it": "Конфликт: слияние остановилось на нём",
  "New, not yet in git": "Новый, ещё не в git",
  "Holds changes": "Внутри есть изменения",
  "Ignored by git": "git его игнорирует",
  "A symbolic link": "Символическая ссылка",
  "Showing the first {n}": "Показаны первые {n}",
  "This folder could not be read: {error}": "Эту папку не удалось прочитать: {error}",
  "This folder is empty.": "Эта папка пуста.",
  "Git could not read this folder: {error}": "git не смог прочитать эту папку: {error}",
  "Show the {n} lines removed here": ["Показать {n} строку, удалённую здесь", "Показать {n} строки, удалённые здесь", "Показать {n} строк, удалённых здесь"],
  "Show the {n} lines it replaced": ["Показать {n} строку, которую оно заменило", "Показать {n} строки, которые оно заменило", "Показать {n} строк, которые оно заменило"],
  "Hide the {n} lines it replaced": ["Скрыть {n} строку, которую оно заменило", "Скрыть {n} строки, которые оно заменило", "Скрыть {n} строк, которые оно заменило"],
};
