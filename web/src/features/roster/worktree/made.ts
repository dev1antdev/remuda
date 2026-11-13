// What the page says once a worktree is made or opened: a toast a line. Pure.

import { t } from "@/lib/i18n";
import type { Made } from "./api";

export function madeWords(made: Made): string[] {
  const out: string[] = [];
  if (made.opened) out.push(t("Its workspace was open already: it is in front now."));
  if (made.setup === "first") out.push(t("The setup script runs first in the new worktree; the agent starts once it is done."));
  if (made.setup === "tab") out.push(t("The setup script runs in the workspace's Setup tab."));
  if (made.warning) out.push(made.warning);
  out.push(...made.notes);
  return out;
}
