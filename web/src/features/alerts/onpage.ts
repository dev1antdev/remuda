// Alerts on the page: while a tab of the cockpit is in front, every new alert the activity records that this device's
// prefs want shows as a toast in the corner — the same alerts a push carries, judged by the same switches, so the
// person at the desk sees them whatever the system's notification settings say. A push meanwhile is taken down by
// the service worker when one of our windows has the focus (public/sw.js), so nothing shows twice.

import type { Entry } from "@/features/activity/api";
import type { Prefs } from "./push";
import { t } from "@/lib/i18n";

// LONG_TURN is the seconds a turn works for "long turns" to tell of its finish, as the server's longTurn
export const LONG_TURN = 60;

// MAX_ON_PAGE is how many alerts stand in the corner at once: a burst after a reconnect shows its newest
export const MAX_ON_PAGE = 3;

// LINGER_MS is how long an alert that needs nothing stands; a question stays until it is opened or dismissed
export const LINGER_MS = 15_000;

// the kinds a schedule or the person's own test send to every device, whatever the watchdog switch says
const ALWAYS = new Set(["schedule", "digest", "test"]);

// the activity's rows that tell of the person's own doing, not of anything that needs them
const NOT_ALERTS = new Set(["sent", "recover", "handover", "teleport"]);

const WARNING = new Set(["input", "stuck", "waiting", "loop", "budget", "limits", "machine"]);

export type OnPageAlert = { id: string; kind: string; text: string; machine?: string; pane?: string; panel?: string; tone: "warn" | "success" | "info"; lingers: boolean };

// wanted says whether this device asks to be told of an entry: a question, a finish by the done switch and the turn's
// length, a schedule's word always, every other alert (the watchdog, the review, a budget, a machine) by the watchdog switch
export function wanted(entry: Entry, prefs: Prefs): boolean {
  if (entry.quiet || NOT_ALERTS.has(entry.kind)) return false;
  if (entry.kind === "input") return prefs.input;
  if (entry.kind === "done") return prefs.done === "always" || (prefs.done === "long" && (entry.worked ?? 0) >= LONG_TURN);
  if (ALWAYS.has(entry.kind)) return true;
  return prefs.watchdog !== false;
}

// newAlerts are the entries past the last one seen that this device wants, oldest first, the newest MAX_ON_PAGE of them
export function newAlerts(entries: Entry[], after: number, prefs: Prefs): Entry[] {
  return entries
    .filter((entry) => entry.id > after && wanted(entry, prefs))
    .sort((a, b) => a.id - b.id)
    .slice(-MAX_ON_PAGE);
}

// alertOf is an entry as a toast: its words, where Open goes, its tone, and whether it goes by itself
export function alertOf(entry: Entry): OnPageAlert {
  const text = entry.body ? `${entry.title} · ${entry.body}` : entry.title;
  const panel = entry.kind === "digest" ? "standup" : undefined;
  return {
    id: `alert-${entry.id}`,
    kind: entry.kind,
    text,
    machine: entry.machine || undefined,
    pane: entry.pane || undefined,
    panel,
    tone: WARNING.has(entry.kind) ? "warn" : entry.kind === "done" || entry.kind === "review" ? "success" : "info",
    lingers: entry.kind !== "input",
  };
}

// testAlert is the toast the alerts panel's test shows on the page itself
export const testAlert = (): OnPageAlert => ({ id: `alert-test-${Date.now()}`, kind: "test", text: t("remuda · Alerts show on this page while it is in front."), tone: "info", lingers: true });

// keepNewest adds an alert to those standing, the same one once, at most MAX_ON_PAGE
export function keepNewest(standing: OnPageAlert[], next: OnPageAlert): OnPageAlert[] {
  if (standing.some((alert) => alert.id === next.id)) return standing;
  return [...standing, next].slice(-MAX_ON_PAGE);
}
