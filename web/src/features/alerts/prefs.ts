import { readPref } from "@/lib/prefs";
import type { Prefs } from "@/features/alerts/push";

export const DEFAULT_ALERT_PREFS: Prefs = { input: true, done: "long", watchdog: true };

export function readAlertPrefs(): Prefs {
  return { ...DEFAULT_ALERT_PREFS, ...readPref<Partial<Prefs>>("alerts", {}) };
}
