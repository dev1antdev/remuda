// The one set of time words: how long ago, when (short), a date — in the page's language (lib/i18n): the words
// through t() and tn(), the dates through the Intl formats of locale().

import { locale, t, tn } from "./i18n";

type When = Date | string | number;
const toDate = (d: When) => (d instanceof Date ? d : new Date(d));
const tag = () => locale() ?? [];

// ago says how long ago: "just now", "5 min ago", "3 h ago", "2 days ago".
export function ago(d: When, now = Date.now()): string {
  const m = Math.round((now - toDate(d).getTime()) / 60000);
  if (m < 1) return t("just now");
  if (m < 60) return t("{n} min ago", { n: m });
  const h = Math.round(m / 60);
  if (h < 48) return t("{n} h ago", { n: h });
  return tn(Math.round(h / 24), "{n} day ago", "{n} days ago");
}

// elapsed says how long something has been going on: "40 s", "6 min", "1 h 12 min", "2 days".
export function elapsed(since: When, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - toDate(since).getTime()) / 1000));
  if (s < 60) return t("{n} s", { n: s });
  const m = Math.round(s / 60);
  if (m < 60) return t("{n} min", { n: m });
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 ? t("{h} h {m} min", { h, m: m % 60 }) : t("{n} h", { n: h });
  return tn(Math.round(h / 24), "{n} day", "{n} days");
}

// spent says how long a piece of work took: "12 s", "4 min 12 s", "1 h 12 min".
export function spent(seconds: number): string {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return t("{n} s", { n: s });
  const m = Math.floor(s / 60);
  if (m < 60) return s % 60 ? t("{m} min {s} s", { m, s: s % 60 }) : t("{n} min", { n: m });
  const h = Math.floor(m / 60);
  return m % 60 ? t("{h} h {m} min", { h, m: m % 60 }) : t("{n} h", { n: h });
}

// took says how long one call took: a tenth under ten seconds ("0.8 s", "1.2 s"), else as spent ("12 s", "4 min 12 s").
export function took(seconds: number): string {
  if (seconds < 10) return t("{n} s", { n: (Math.round(Math.max(0, seconds) * 10) / 10).toFixed(1) });
  return spent(seconds);
}

// whenShort is the time if today, the weekday and time within a week, else the date.
export function whenShort(d: When): string {
  const t = toDate(d);
  const now = new Date();
  const time = t.toLocaleTimeString(tag(), { hour: "2-digit", minute: "2-digit" });
  if (t.toDateString() === now.toDateString()) return time;
  const days = Math.round((now.getTime() - t.getTime()) / 86400000);
  if (days >= 0 && days < 7) return `${t.toLocaleDateString(tag(), { weekday: "short" })} ${time}`;
  return t.toLocaleDateString(tag(), { day: "numeric", month: "short", year: t.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

// fmtDate: "7 Oct 2026".
export const fmtDate = (d: When) => toDate(d).toLocaleDateString(tag(), { day: "numeric", month: "short", year: "numeric" });

// fmtDay: "7 Oct" from a YYYY-MM-DD key.
export function fmtDay(iso: string): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y!, m! - 1, d!).toLocaleDateString(tag(), { day: "numeric", month: "short" });
}
