// The words under an agent key: what it opens, when it was last used, since when.

import { ago, fmtDay } from "@/lib/time";
import type { AgentKey } from "./api";
import { t } from "@/lib/i18n";

// keyLine is "tasks and reading · used 3 h ago · since Oct 26": what the key opens, its last use, its first day
export function keyLine(key: Pick<AgentKey, "scope" | "created" | "last_used">, now = Date.now()): string {
  const parts = [key.scope === "tasks" ? "tasks and reading" : key.scope];
  parts.push(key.last_used === key.created ? "never used" : `used ${ago(key.last_used, now)}`);
  parts.push(`since ${fmtDay(key.created.slice(0, 10))}`); // fmtDay takes the day alone, YYYY-MM-DD
  return parts.join(" · ");
}

// revokeKeyQuestion is the confirm before a key is revoked: the machine's agents lose the tasks until remuda hands it a new one
export function revokeKeyQuestion(key: Pick<AgentKey, "name">): { text: string; label: string } {
  return { text: `Revoke the key of “${key.name}”? Its agents lose the tasks until remuda connects to it again and hands it a new one.`, label: t("Revoke") };
}
