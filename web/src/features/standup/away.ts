// While you were away: the gap since the page was last seen, and what the agents did in it, read off the standup since
// that moment — how many answered, how many wait for the person, what it cost. The pure part; useWelcomeBack shows it.

import { usd } from "@/features/usage/format";
import { plural } from "@/lib/text";
import type { Item } from "./api";

export const LAST_SEEN_KEY = "page.lastSeen";
export const AWAY_AFTER = 3600_000;

// awayGap is how long the page was away in milliseconds, nothing under an hour, without a stamp or with one that does not parse
export function awayGap(lastSeen: string | null | undefined, now: number): number | null {
  if (!lastSeen) return null;
  const then = Date.parse(lastSeen);
  if (!Number.isFinite(then)) return null;
  const gap = now - then;
  return gap >= AWAY_AFTER ? gap : null;
}

// awayWords says how long: hours under a day, days from there on
export function awayWords(gap: number): string {
  const hours = Math.floor(gap / 3600_000);
  if (hours < 24) return `${hours} h`;
  return plural(Math.round(hours / 24), "day");
}

export type AwayCounts = { answered: number; waiting: number; cost: number };

// awayCounts sums the standup up: the agents that answered since, the ones waiting for the person now, the cost of it all
export function awayCounts(items: Item[]): AwayCounts {
  const counts: AwayCounts = { answered: 0, waiting: 0, cost: 0 };
  for (const item of items) {
    if (item.recap.answers > 0) counts.answered++;
    if (item.alert || item.state === "blocked" || item.recap.asking) counts.waiting++;
    counts.cost += item.recap.cost;
  }
  return counts;
}

// awayLine is the one line of the toast, nothing when nothing happened
export function awayLine(items: Item[], gap: number): string | null {
  const { answered, waiting, cost } = awayCounts(items);
  const parts: string[] = [];
  if (answered > 0) parts.push(`${plural(answered, "agent")} answered`);
  if (waiting > 0) parts.push(`${waiting} ${waiting === 1 ? "waits" : "wait"} for you`);
  if (cost > 0) parts.push(`${usd(cost)} spent`);
  if (!parts.length) return null;
  return `Away ${awayWords(gap)}: ${parts.join(", ")}`;
}
