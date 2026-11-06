// The Russian words the chat's audit of 0.12.0 found still in English: the composer, the find bar, the files rail,
// the chat's notices, the quick prompts, the file viewer's cut and a diagram whose library did not load.

import type { Dictionary } from "../../i18n";

export const auditchat: Dictionary = {
  "Attach files (up to {n}): they are uploaded into the pane's folder": "Прикрепить файлы (до {n}): они загружаются в папку панели",
  "Uploading…": "Загружаю…",
  "Send (Enter)": "Отправить (Enter)",
  "Previous match (Shift+Enter)": "Предыдущее совпадение (Shift+Enter)",
  "Next match (Enter)": "Следующее совпадение (Enter)",
  "Find {what} ({shortcut})": "Найти {what} ({shortcut})",
  "Open in Code (VS Code on this machine)": "Открыть в Code (VS Code на этой машине)",
  "Context {pct}% full — Claude Code compacts on its own near the edge; compacting now keeps the summary yours to steer.":
    "Контекст заполнен на {pct}% — у края Claude Code сожмёт его сам; если сжать сейчас, сводкой управляете вы.",
  "That turn is older than the loaded part of this transcript (the last 16 MB).": "Этот ход старше загруженной части переписки (последних 16 МБ).",
  "… cut at {cut} of {size}": "… обрезано на {cut} из {size}",
  "Offered under": "Предлагаются под",
  "in the composer; one tap puts the text in. Shared by every device that uses this remuda.":
    "в поле ввода; одно нажатие вставляет текст. Общие для всех устройств этого remuda.",
  and: "и",
  "become the pane's folder, its branch and the file you last opened.": "заменяются папкой панели, её веткой и последним открытым файлом.",
  "only {folder}": "только {folder}",
  "its library did not load — reload the page": "её библиотека не загрузилась — перезагрузите страницу",
};
