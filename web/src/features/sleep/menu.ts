// The workspace menu's row for its sleep: Wake while it sleeps, else Sleep… where an agent runs.

import type { Machine, Session } from "@/features/roster/api";
import { t, tn } from "@/lib/i18n";
import type { MenuItem } from "@/ui";
import { askSleep, wake } from "./flows";
import { canSleep } from "./sleep";

export function sleepRows(m: Machine, session: Session): MenuItem[] {
  const disabled = m.state !== "connected";
  if (session.asleep)
    return [{ label: t("Wake"), icon: "sun", hint: tn(session.asleep.agents, "{n} agent back on its conversation", "{n} agents back on their conversations"), disabled, onSelect: () => void wake(m.id, session) }];
  if (!canSleep(session)) return [];
  return [{ label: t("Sleep…"), icon: "moon", hint: t("its agents leave, their shells stay; waking resumes them"), disabled, onSelect: () => askSleep(m.id, session) }];
}
