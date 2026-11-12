// Alerts on the page: while a tab of the cockpit is in front, every new alert the activity records that this device's
// prefs want shows as a toast in the corner — the same alerts a push carries, judged by the same switches, so the
// person at the desk sees them whatever the system's notification settings say. A push meanwhile is taken down by
// the service worker when one of our windows has the focus (public/sw.js); a window on screen without the focus gets
// both, the system's one quiet. A question's toast leaves once its pane no longer asks, wherever it was answered.

import type { Entry } from "@/features/activity/api";
import type { Prefs } from "./push";
import { t } from "@/lib/i18n";

// LONG_TURN is the seconds a turn works for "long turns" to tell of its finish, as the server's longTurn
export const LONG_TURN = 60;

// MAX_ON_PAGE is how many alerts stand in the corner at once: a burst after a reconnect shows its newest
export const MAX_ON_PAGE = 3;

// LINGER_MS is how long an alert that needs nothing stands; a question stays until it is opened, dismissed or answered
export const LINGER_MS = 15_000;

// the kinds the server sends every device whatever the watchdog switch says (push.Broadcast: a schedule's word, a
// digest, a backup's failure) and the person's own test
const ALWAYS = new Set(["schedule", "digest", "backup", "test"]);

// the activity's rows that tell of the person's own doing, not of anything that needs them
const NOT_ALERTS = new Set(["sent", "recover", "handover", "teleport"]);

const WARNING = new Set(["input", "stuck", "waiting", "loop", "budget", "limits", "machine", "backup", "tests"]);

// the panel Open goes to for an alert that names no pane
const PANEL_OF: Record<string, string> = { digest: "standup", budget: "usage", limits: "usage" };

// OnPageAlert is a toast's alert, its title and body apart so the screen-sharing mode can keep a hidden folder's words out
export type OnPageAlert = { id: string; kind: string; title: string; body?: string; machine?: string; pane?: string; panel?: string; tone: "warn" | "success" | "info"; lingers: boolean; asked?: boolean }; // asked: its pane was seen asking

// wanted says whether this device asks to be told of an entry: a question, a finish by the done switch and the turn's
// length, the server's broadcasts always, every other alert (the watchdog, the review, a budget, a machine) by the watchdog switch
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
  return {
    id: `alert-${entry.id}`,
    kind: entry.kind,
    title: entry.title,
    body: entry.body || undefined,
    machine: entry.machine || undefined,
    pane: entry.pane || undefined,
    panel: entry.pane ? undefined : PANEL_OF[entry.kind],
    tone: WARNING.has(entry.kind) ? "warn" : entry.kind === "done" || entry.kind === "review" ? "success" : "info",
    lingers: entry.kind !== "input",
  };
}

// alertText is a toast's line: the title, then the body when there is one
export const alertText = (alert: Pick<OnPageAlert, "title" | "body">): string => (alert.body ? `${alert.title} · ${alert.body}` : alert.title);

// testAlert is the toast the alerts panel's test shows on the page itself
export const testAlert = (): OnPageAlert => ({ id: `alert-test-${Date.now()}`, kind: "test", title: "remuda", body: t("Alerts show on this page while it is in front."), tone: "info", lingers: true });

// keepNewest adds an alert to those standing, the same one once, at most MAX_ON_PAGE
export function keepNewest(standing: OnPageAlert[], next: OnPageAlert): OnPageAlert[] {
  if (standing.some((alert) => alert.id === next.id)) return standing;
  return [...standing, next].slice(-MAX_ON_PAGE);
}

// stillStanding are the alerts to keep up: a question's toast goes once its pane no longer asks — answered at the
// terminal, from a phone, in another tab — or is gone; one whose pane was never seen asking is a note about the pane
// (a first prompt not delivered), which goes like any other alert; the same array back when nothing moved
export function stillStanding(alerts: OnPageAlert[], asks: (machine: string, pane: string) => boolean): OnPageAlert[] {
  let moved = false;
  const kept: OnPageAlert[] = [];
  for (const alert of alerts) {
    if (alert.kind !== "input" || !alert.machine || !alert.pane) kept.push(alert);
    else if (asks(alert.machine, alert.pane)) {
      kept.push(alert.asked ? alert : { ...alert, asked: true });
      moved ||= !alert.asked;
    } else if (!alert.asked) {
      kept.push(alert.lingers ? alert : { ...alert, lingers: true });
      moved ||= !alert.lingers;
    } else moved = true;
  }
  return moved ? kept : alerts;
}

// minutesIn is a moment's clock in a zone as minutes past midnight; an unknown zone reads as UTC, as the server's
function minutesIn(now: Date, tz: string): number {
  const read = (zone: string) => new Intl.DateTimeFormat("en-GB", { timeZone: zone, hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).formatToParts(now);
  let parts: Intl.DateTimeFormatPart[];
  try {
    parts = read(tz || "UTC");
  } catch {
    parts = read("UTC");
  }
  const value = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? 0);
  return value("hour") * 60 + value("minute");
}

// quietNow says whether the device's quiet window covers a moment, as the server's Quiet.Covers: a window past
// midnight wraps, equal times are no window
export function quietNow(quiet: Prefs["quiet"], now = new Date()): boolean {
  if (!quiet) return false;
  const minuteOf = (clock: string) => {
    const [hour, minute] = clock.split(":").map(Number);
    return (hour ?? 0) * 60 + (minute ?? 0);
  };
  const from = minuteOf(quiet.from);
  const to = minuteOf(quiet.to);
  if (from === to) return false;
  const minute = minutesIn(now, quiet.tz);
  return from < to ? minute >= from && minute < to : minute >= from || minute < to;
}
