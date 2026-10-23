// The activity list by day — Today, Yesterday, then dates — newest first within each; pure, tested.

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
  if (at.toDateString() === today.toDateString()) return "Today";
  if (at.toDateString() === yesterday.toDateString()) return "Yesterday";
  return fmtDate(at);
}

// unread says whether the feed has entries this device has not opened the panel for
export const unread = (newest: number | undefined, seen: number): boolean => (newest ?? 0) > seen;
