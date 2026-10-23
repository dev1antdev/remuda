import { readPref } from "@/lib/prefs";
import type { Prefs, Quiet } from "@/features/alerts/push";

export const DEFAULT_ALERT_PREFS: Prefs = { input: true, done: "long", watchdog: true };

// DEFAULT_QUIET is the window offered when quiet hours are first turned on.
export const DEFAULT_QUIET: Omit<Quiet, "tz"> = { from: "22:00", to: "07:00" };

// deviceZone is this device's IANA zone, re-read on every load so a window set at home still fits the clock on a trip.
export const deviceZone = (): string => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

// readAlertPrefs is what this device asks of the server; a quiet window always carries the zone of the moment.
export function readAlertPrefs(): Prefs {
  const prefs = { ...DEFAULT_ALERT_PREFS, ...readPref<Partial<Prefs>>("alerts", {}) };
  if (prefs.quiet) prefs.quiet = { ...prefs.quiet, tz: deviceZone() };
  return prefs;
}

// readQuietWindow is the window last set on this device, kept while quiet hours are off so turning them on again brings it back.
export const readQuietWindow = (): Omit<Quiet, "tz"> => readPref("alerts.quiet", DEFAULT_QUIET);
