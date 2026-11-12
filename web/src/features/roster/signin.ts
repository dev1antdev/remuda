// Claude Code's sign-in on a machine running out, as the vitals read it (`claude auth status`): a look from ten days
// before the end, bad from two days and once it has ended, saying where to run `claude auth login` — remuda never
// signs in for the person; the ⓘ's button opens a shell there.

import { t, tn } from "@/lib/i18n";
import { ago, fmtDate } from "@/lib/time";

// Login is when the stored sign-in ends and when that was read
export type Login = { expires: string; at: string };

// LOOK_DAYS and BAD_DAYS are how long before the end the concern turns amber, then red
export const LOOK_DAYS = 10;
export const BAD_DAYS = 2;
const DAY = 86_400_000;

// endsAt is the end of the sign-in in ms, NaN when none is known
const endsAt = (login: Login | undefined): number => (login?.expires ? Date.parse(login.expires) : NaN);

// leftWords says how long is left, never rounded up: days from two days, hours under them, minutes under an hour
function leftWords(ms: number): string {
  const minutes = Math.max(1, Math.floor(ms / 60_000));
  if (minutes < 60) return t("{n} min", { n: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return t("{n} h", { n: hours });
  return tn(Math.floor(hours / 24), "{n} day", "{n} days");
}

// signinConcern is the ⓘ's word on the sign-in: how long is left or how long ago it ended, and what to run where;
// null while more than LOOK_DAYS are left or no end is known
export function signinConcern(login: Login | undefined, machine: string, now: number): { text: string; bad: boolean } | null {
  const ends = endsAt(login);
  if (!Number.isFinite(ends) || ends - now > LOOK_DAYS * DAY) return null;
  if (ends <= now) return { text: t("Claude Code's sign-in on {machine} ended {ago}: its agents cannot reach Claude until you run claude auth login there", { machine, ago: ago(ends, now) }), bad: true };
  return { text: t("Claude Code's sign-in on {machine} ends in {left}: run claude auth login there before then", { machine, left: leftWords(ends - now) }), bad: ends - now <= BAD_DAYS * DAY };
}

// loginWords is the vitals' row: the day it ends and how long is left, or that it has ended; "" when no end is known
export function loginWords(login: Login | undefined, now: number): string {
  const ends = endsAt(login);
  if (!Number.isFinite(ends)) return "";
  if (ends <= now) return t("ended {date}", { date: fmtDate(ends) });
  return t("ends {date} · in {left}", { date: fmtDate(ends), left: leftWords(ends - now) });
}
