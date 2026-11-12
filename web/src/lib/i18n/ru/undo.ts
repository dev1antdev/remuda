// The Russian words of undoing an agent's turn: the dialog, the Changes lens's button and an answer's foot in the chat.

import type { Dictionary } from "../../i18n";

export const undo: Dictionary = {
  "Undo a turn": "Отменить ход",
  "Undo a turn…": "Отменить ход…",
  "Undo this turn": "Отменить этот ход",
  "Undoing…": "Отменяю…",
  "Put the files back as they were before an agent's turn: the last one, or one you pick": "Вернуть файлы к тому, какими они были до хода агента: последнего или выбранного вами",
  "Put the files back as they were before this turn; you see what changes first": "Вернуть файлы к тому, какими они были до этого хода; сначала вы увидите, что изменится",
  "Puts the files back as they were when the agent's turn began. A commit it made is reverted with a new commit, so nothing is lost. The agent is told nothing.":
    "Файлы вернутся к тому, какими были в начале хода агента. Сделанный им коммит отменяется новым коммитом, так что ничего не теряется. Агенту ничего не сообщается.",
  Turn: "Ход",
  "No agent's turn has changed anything in this folder yet.": "Ни один ход агента пока ничего не изменил в этой папке.",
  "This remuda keeps no turns: a second instance beside the real one leaves them to it.": "Этот remuda не хранит ходы: второй экземпляр рядом с настоящим оставляет их ему.",
  "Reading the folder…": "Читаю папку…",
  "Another agent worked in this folder during this turn: its changes are undone too.": "Во время этого хода в папке работал ещё один агент: его изменения тоже отменятся.",
  binary: "двоичный",
  "Also revert the {commits} it made: git revert, a new commit each, newest first": "Отменить и коммиты хода ({commits}): git revert, по новому коммиту на каждый, начиная с последнего",
  "a merge": "слияние",
  "Without the reverts, {files} stay as the commits have them.": "Без отмены коммитов не вернутся файлы ({files}): их изменения в коммитах.",
  "An agent": "Агент",
  undone: "отменён",
  "cannot be undone": "нельзя отменить",
  "left as it is: only reverting the commits puts it back": "останется как есть: вернёт только отмена коммитов",
  "stays as the commits have it": "останется как в коммитах",
  "back to the last commit": "вернётся к последнему коммиту",
  "removed: the turn made it": "удалится: его создал ход",
  "brought back": "вернётся",
  "its mode put back": "вернутся права доступа",
  "put back": "вернётся как было",
  "Undone: {files} put back, {commits} reverted": "Готово: возвращено — {files}, отменено — {commits}",
  "Undone: {commits} reverted": "Готово: отменено — {commits}",
  "Undone: {files} put back": "Готово: возвращено — {files}",
  "Already pushed to {branches}: the reverts are new commits, to be pushed like any other.": "Уже отправлено в {branches}: отмены — это новые коммиты, их нужно будет отправить, как любые другие.",
};
