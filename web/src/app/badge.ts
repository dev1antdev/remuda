// The browser tab's badge: the title counts the agents that need the person — the Wall's Needs-me rule, a question open,
// a watchdog note, a finish not looked at — and the favicon wears the colour of the most pressing one: red for a
// question, amber for a note, green for a finish. The pure part; useBadge draws it.

import type { Machine, Pane } from "@/features/roster/api";
import { displayOf, type RadarPrefs } from "@/features/roster/radar";
import { needsMe } from "@/features/roster/wall";

export type BadgeTone = "red" | "amber" | "green";
export type Badge = { count: number; tone: BadgeTone | null };

export const BASE_TITLE = "remuda";
const RANK: Record<BadgeTone, number> = { red: 3, amber: 2, green: 1 };

// titleFor is the tab's title: the count in front while anyone needs the person
export function titleFor(count: number): string {
  return count > 0 ? `(${count}) ${BASE_TITLE}` : BASE_TITLE;
}

// toneOf is the colour one agent asks for, nothing when it needs nobody
export function toneOf(pane: Pane, radar: RadarPrefs, now: number): BadgeTone | null {
  if (!pane.agent || !needsMe(pane, radar, now)) return null;
  const display = displayOf(pane, radar, now);
  if (display === "blocked") return "red";
  if (pane.radar?.alert) return "amber";
  return "green";
}

// pressing is the more urgent of two tones
export function pressing(a: BadgeTone | null, b: BadgeTone | null): BadgeTone | null {
  if (!a) return b;
  if (!b) return a;
  return RANK[b] > RANK[a] ? b : a;
}

// badgeOf counts the agents on connected machines that need the person and the tone of the most pressing of them
export function badgeOf(machines: Machine[], radar: RadarPrefs, now: number): Badge {
  const badge: Badge = { count: 0, tone: null };
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const session of machine.sessions)
      for (const tab of session.tabs)
        for (const pane of tab.panes) {
          const tone = toneOf(pane, radar, now);
          if (!tone) continue;
          badge.count++;
          badge.tone = pressing(badge.tone, tone);
        }
  }
  return badge;
}

// sameBadge says whether two readings would draw the same tab
export const sameBadge = (a: Badge, b: Badge): boolean => a.count === b.count && a.tone === b.tone;
