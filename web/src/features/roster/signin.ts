// Claude Code's sign-in on a machine running out, as the vitals read it (`claude auth status`): Claude Code names the
// latest moment it ends and says it may end up to EARLY_DAYS before, so the look comes from LOOK_DAYS before that
// moment (when Claude Code starts warning itself), bad from BAD_DAYS before the earliest end and from then on, saying
// where to run `claude auth login` — remuda never signs in for the person; the ⓘ's button opens a shell there.

import { t, tn } from "@/lib/i18n";
import { ago, fmtDate, fmtDateSpan } from "@/lib/time";

// Login is when the stored sign-in ends at the latest and when that was read
export type Login = { expires: string; at: string };

// EARLY_DAYS is how long before the end named the sign-in may already end: Claude Code says it "ends between" the two
export const EARLY_DAYS = 3;
// LOOK_DAYS is how long before the end named the concern turns amber; BAD_DAYS how long before the earliest end it turns red
export const LOOK_DAYS = 10;
export const BAD_DAYS = 2;
const DAY = 86_400_000;

// endsAt is the latest end of the sign-in in ms, NaN when none is known
const endsAt = (login: Login | undefined): number => (login?.expires ? Date.parse(login.expires) : NaN);

// leftWords says how long is left, never rounded up: days from two days, hours under them, minutes under an hour
function leftWords(ms: number): string {
  const minutes = Math.max(1, Math.floor(ms / 60_000));
  if (minutes < 60) return t("{n} min", { n: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return t("{n} h", { n: hours });
  return tn(Math.floor(hours / 24), "{n} day", "{n} days");
}

// signinConcern is the ⓘ's word on the sign-in: how long is left to its earliest end, that it may end at any time, or
// how long ago it ended, and what to run where; null while more than LOOK_DAYS are left or no end is known
export function signinConcern(login: Login | undefined, machine: string, now: number): { text: string; bad: boolean } | null {
  const ends = endsAt(login);
  if (!Number.isFinite(ends) || ends - now > LOOK_DAYS * DAY) return null;
  if (ends <= now) return { text: t("Claude Code's sign-in on {machine} ended {ago}: its agents cannot reach Claude until you run claude auth login there", { machine, ago: ago(ends, now) }), bad: true };
  const earliest = ends - EARLY_DAYS * DAY;
  const date = fmtDate(ends);
  if (earliest <= now) return { text: t("Claude Code's sign-in on {machine} may end at any time now, by {date} at the latest: run claude auth login there", { machine, date }), bad: true };
  return { text: t("Claude Code's sign-in on {machine} may end in {left}, by {date} at the latest: run claude auth login there before then", { machine, left: leftWords(earliest - now), date }), bad: earliest - now <= BAD_DAYS * DAY };
}

// loginWords is the vitals' row: the days it ends between and how long is left to the first, that it may end at any
// time, or that it has ended; "" when no end is known
export function loginWords(login: Login | undefined, now: number): string {
  const ends = endsAt(login);
  if (!Number.isFinite(ends)) return "";
  if (ends <= now) return t("ended {date}", { date: fmtDate(ends) });
  const earliest = ends - EARLY_DAYS * DAY;
  if (earliest <= now) return t("may end any time now · by {date}", { date: fmtDate(ends) });
  return t("ends {days} · in {left} at the earliest", { days: fmtDateSpan(earliest, ends), left: leftWords(earliest - now) });
}
