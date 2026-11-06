// The Russian words of the backups: the Settings section, the webhook kind, the last run and the next.

import type { Dictionary } from "../../i18n";

export const backup: Dictionary = {
  Backups: "Резервные копии",
  "Every night the cockpit's state — the tasks, the schedules, the budgets, the recipes, the webhooks, the settings, the signed-in devices, the agents' keys, the token, the push keys and its Tailscale node — is copied to the machine picked here, into its remuda folder under backups/{host}. `remuda restore` puts a copy back: on a new machine the cockpit keeps its address and every device stays signed in. Never run the old cockpit and the restored one at once.":
    "Каждую ночь состояние кабины — задачи, расписания, бюджеты, рецепты, вебхуки, настройки, вошедшие устройства, ключи агентов, токен, ключи пушей и её узел Tailscale — копируется на выбранную здесь машину, в её папку remuda, в backups/{host}. `remuda restore` возвращает копию: на новой машине кабина сохранит свой адрес, и все устройства останутся залогинены. Никогда не запускайте старую кабину и восстановленную одновременно.",
  "pick another machine than the cockpit's own: a dead disk takes its own backups with it": "выберите не ту машину, где работает кабина: умерший диск унесёт с собой и свои копии",
  "No backups": "Без копий",
  "Every night at": "Каждую ночь в",
  "the cockpit's own time": "по времени кабины",
  "Copies kept": "Хранить копий",
  "the newest; older ones are removed on the machine": "последних; старые удаляются на машине",
  "Backing up…": "Копирую…",
  "Back up now": "Сделать копию сейчас",
  "{n} copies in {folder}": ["{n} копия в {folder}", "{n} копии в {folder}", "{n} копий в {folder}"],
  "No copy in {folder} yet": "В {folder} пока нет копий",
  "Failed {when}, to {machine}: {error}": "Не удалось {when}, на {machine}: {error}",
  "Taken {when} to {machine} · {size} · {files} files": "Сделана {when} на {machine} · {size} · файлов: {files}",
  "the newest good copy is from {when}": "последняя удачная копия — {when}",
  "no good copy yet": "удачной копии пока нет",
  "next today at {time}": "следующая сегодня в {time}",
  "next tomorrow at {time}": "следующая завтра в {time}",
  "next on {date}": "следующая {date}",
  "a backup that failed": "неудавшаяся резервная копия",
  "the nightly copy of the cockpit's state, or Back up now, could not be made": "ночная копия состояния кабины или «Сделать копию сейчас» не удалась",
};
