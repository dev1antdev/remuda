// The Russian words of a commit and a push past the repository's git hooks (--no-verify).

import type { Dictionary } from "../../i18n";

export const noverify: Dictionary = {
  "Skip hooks (--no-verify)": "Без хуков (--no-verify)",
  "the pre-commit and commit-msg hooks do not run": "хуки pre-commit и commit-msg не запускаются",
  "The pre-commit and commit-msg hooks will not run (--no-verify).": "Хуки pre-commit и commit-msg не запустятся (--no-verify).",
  "hooks skipped": "без хуков",
  "Push without hooks": "Push без хуков",
  "Push and set upstream without hooks": "Push и задать upstream без хуков",
  "--no-verify: the pre-push hook does not run": "--no-verify: хук pre-push не запускается",
  "{branch} sent with --no-verify: the pre-push hook does not run": "{branch} отправляется с --no-verify: хук pre-push не запускается",
};
