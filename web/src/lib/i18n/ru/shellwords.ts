// The Russian words of the shell's and the pieces' strings that were still outside t(): the header's search and usage
// tooltips, the Wall's counts and office button, a tile's tooltips, the transcript viewer's bar, a code fence's bar, the
// stall toast's place and a palette row.

import type { Dictionary } from "../../i18n";

export const shellwords: Dictionary = {
  "Search every conversation on every machine (⌘K / Ctrl+K has it too)": "Искать во всех разговорах на всех машинах (то же есть в ⌘K / Ctrl+K)",
  "{model}: context {used} of {window} ({pct}%) · {responses} · {cost} API-equivalent so far.": "{model}: контекст {used} из {window} ({pct}%) · {responses} · пока {cost} по ценам API.",
  "Budget: {words}.": "Бюджет: {words}.",
  "Click for the usage report.": "Нажмите, чтобы открыть отчёт о расходах.",
  "Show {n} hidden": "Показать скрытые: {n}",
  "Leave office mode (Esc)": "Выйти из Office-режима (Esc)",
  "Back to the grid (Esc)": "Назад к сетке (Esc)",
  "A subagent of": "Субагент из",
  "— its own conversation, read-only.": "— его собственный разговор, только чтение.",
  "A transcript from": "Расшифровка из",
  code: "код",
  copy: "копировать",
  copied: "скопировано",
  "on the Wall ({tiles})": "на Стене ({tiles})",
  "Rename tab {tab}…": "Переименовать вкладку {tab}…",
};
