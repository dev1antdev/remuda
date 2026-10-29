// The words under a viewer link: when it was last used and since when, and the confirm before it is revoked.

import { ago, fmtDay } from "@/lib/time";
import type { ViewerKey } from "./api";
import { t } from "@/lib/i18n";

// viewerLine is "used 3 h ago · since Oct 26", or "never used · since Oct 26" for a link nobody opened yet
export function viewerLine(key: Pick<ViewerKey, "created" | "last_used">, now = Date.now()): string {
  const used = key.last_used === key.created ? t("never used") : t("used {ago}", { ago: ago(key.last_used, now) });
  return `${used} · ${t("since {day}", { day: fmtDay(key.created.slice(0, 10)) })}`;
}

// revokeViewerQuestion is the confirm before a link is revoked: every screen it signed in goes blank at once
export function revokeViewerQuestion(key: Pick<ViewerKey, "name">): { text: string; label: string } {
  return { text: t("Revoke the link “{name}”? Every screen it signed in is signed out at once.", { name: key.name }), label: t("Revoke") };
}
