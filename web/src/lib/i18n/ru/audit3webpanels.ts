// The Russian words the third audit of the panels added: the project settings' save guard and the words that were left
// outside t() in the sidebar, the tasks and the updates.

import type { Dictionary } from "../../i18n";

export const audit3webpanels: Dictionary = {
  "The queue needs a machine and a repository, or switch it off": "Очереди нужны машина и репозиторий — или выключите её",
  "The queue needs a column marked Queued, or switch it off": "Очереди нужна колонка «В очереди» — или выключите её",
  "Stage {n} needs a prompt": "Этапу {n} нужен промпт",
  "{what} {version} is out ({have} here)": "Вышел {what} {version} (здесь {have})",
  "code-server {version} is what remuda installs now ({have} here)": "remuda теперь ставит code-server {version} (здесь {have})",
  Deploy: "Развернуть",
  Restart: "Перезапустить",
  Update: "Обновить",
  "{n} moved": ["перемещено: {n}", "перемещено: {n}", "перемещено: {n}"],
  "{n} labelled": ["с меткой: {n}", "с меткой: {n}", "с меткой: {n}"],
  "{n} marked done": ["отмечено готовым: {n}", "отмечено готовым: {n}", "отмечено готовым: {n}"],
  "{n} deleted": ["удалено: {n}", "удалено: {n}", "удалено: {n}"],
  "Clear the selection (Esc)": "Снять выделение (Esc)",
  "asking — would refuse": "спрашивает — откажется",
  sent: "отправлено",
  "Pick a recipe": "Выберите рецепт",
  "A folder is required": "Нужна папка",
  "SSH host is required": "Нужен SSH-хост",
  "This remuda cannot be deployed there: that needs a remote machine of the same OS and CPU, and a released build here.":
    "Этот remuda туда не развернуть: нужна удалённая машина с той же ОС и процессором и выпущенная сборка здесь.",
  "No newer release of remuda is known for this machine.": "Более нового выпуска remuda для этой машины не известно.",
  "herdr's hooks there are current.": "Хуки herdr там свежие.",
  "No herdr plugin from GitHub is installed there, or herdr there is older than 0.9.4.": "Там не установлено ни одного плагина herdr с GitHub, или herdr там старше 0.9.4.",
  "Done. remuda is restarting here; the page offers a reload once the new version answers.": "Готово. remuda здесь перезапускается; страница предложит перезагрузку, когда новая версия ответит.",
};
