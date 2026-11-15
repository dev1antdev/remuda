// The Russian words the fourth audit of the git client added: a push anyway refused for commits made since its check.

import type { Dictionary } from "../../i18n";

export const audit4vcs: Dictionary = {
  "{name} has new commits since its secret check: nothing was pushed. Push again to check them too.":
    "В {name} появились новые коммиты после проверки на секреты: ничего не отправлено. Нажмите Push ещё раз, чтобы проверить и их.",
};
