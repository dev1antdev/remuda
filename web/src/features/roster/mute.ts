// A workspace muted for a while: the choices the ⋯ menu offers, the words a muted one wears, and whether the roster
// says a workspace is muted right now. The server keeps the mute; this is the page's reading of it.

import { t } from "@/lib/i18n";
import type { Session } from "./api";

export type MuteChoice = { key: "hour" | "morning"; label: string; until: Date };

const MORNING_HOUR = 9;

// muteChoices is what the menu offers: an hour from now, and the next 09:00 that is at least an hour away
export function muteChoices(now: Date = new Date()): MuteChoice[] {
  const hour = new Date(now.getTime() + 3_600_000);
  const morning = new Date(now);
  morning.setHours(MORNING_HOUR, 0, 0, 0);
  if (morning.getTime() - now.getTime() < 3_600_000) morning.setDate(morning.getDate() + 1);
  const tomorrow = morning.getDate() !== now.getDate();
  return [
    { key: "hour", label: t("Mute for an hour"), until: hour },
    { key: "morning", label: tomorrow ? t("Mute until tomorrow 09:00") : t("Mute until 09:00"), until: morning },
  ];
}

// mutedUntil is the moment a workspace's silence ends, null when it is not muted or the moment has passed
export function mutedUntil(session: Pick<Session, "muted_until">, now: Date = new Date()): Date | null {
  if (!session.muted_until) return null;
  const until = new Date(session.muted_until);
  return until.getTime() > now.getTime() ? until : null;
}

// leftWords says how long a mute still holds: "42 min left", "3 h 10 min left", "until Fri 09:00" past a day
export function leftWords(until: Date, now: Date = new Date()): string {
  const minutes = Math.max(1, Math.round((until.getTime() - now.getTime()) / 60_000));
  if (minutes < 60) return t("{n} min left", { n: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return minutes % 60 ? t("{h} h {m} min left", { h: hours, m: minutes % 60 }) : t("{n} h left", { n: hours });
  return t("until {when}", { when: until.toLocaleString(undefined, { weekday: "short", hour: "2-digit", minute: "2-digit" }) });
}

// muteWords is the mark's tooltip: when the silence ends and what it silences
export function muteWords(until: Date, now: Date = new Date()): string {
  return t("Muted · {left} · no alerts, webhooks or Activity rows from this workspace until then", { left: leftWords(until, now) });
}
