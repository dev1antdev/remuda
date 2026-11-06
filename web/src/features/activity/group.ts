// The activity list by day — Today, Yesterday, then dates — newest first within each; pure, tested.

import { t } from "@/lib/i18n";
import { fmtDate } from "@/lib/time";
import type { Entry } from "./api";

export type Day = { label: string; entries: Entry[] };

export function groupByDay(entries: Entry[], now = Date.now()): Day[] {
  const days: Day[] = [];
  for (const entry of entries) {
    const label = dayLabel(new Date(entry.at), now);
    const last = days[days.length - 1];
    if (last && last.label === label) last.entries.push(entry);
    else days.push({ label, entries: [entry] });
  }
  return days;
}

function dayLabel(at: Date, now: number): string {
  const today = new Date(now);
  const yesterday = new Date(now - 86_400_000);
  if (at.toDateString() === today.toDateString()) return t("Today");
  if (at.toDateString() === yesterday.toDateString()) return t("Yesterday");
  return fmtDate(at);
}

// unread says whether the feed has entries this device has not opened the panel for; a newest id below the one seen
// means the cockpit's activity started over (a new state folder, another machine at the same address), all of it news
export const unread = (newest: number | undefined, seen: number): boolean => (newest ?? 0) > 0 && (newest ?? 0) !== seen;
