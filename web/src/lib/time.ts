// The one set of time words: how long ago, when (short), a date.

type When = Date | string | number;
const toDate = (d: When) => (d instanceof Date ? d : new Date(d));

// ago says how long ago: "just now", "5 min ago", "3 h ago", "2 days ago".
export function ago(d: When, now = Date.now()): string {
  const m = Math.round((now - toDate(d).getTime()) / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}

// elapsed says how long something has been going on: "40 s", "6 min", "1 h 12 min", "2 days".
export function elapsed(since: When, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - toDate(since).getTime()) / 1000));
  if (s < 60) return `${s} s`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 ? `${h} h ${m % 60} min` : `${h} h`;
  return `${Math.round(h / 24)} days`;
}

// spent says how long a piece of work took: "12 s", "4 min 12 s", "1 h 12 min".
export function spent(seconds: number): string {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return s % 60 ? `${m} min ${s % 60} s` : `${m} min`;
  const h = Math.floor(m / 60);
  return m % 60 ? `${h} h ${m % 60} min` : `${h} h`;
}

// whenShort is the time if today, the weekday and time within a week, else the date.
export function whenShort(d: When): string {
  const t = toDate(d);
  const now = new Date();
  const time = t.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  if (t.toDateString() === now.toDateString()) return time;
  const days = Math.round((now.getTime() - t.getTime()) / 86400000);
  if (days >= 0 && days < 7) return `${t.toLocaleDateString([], { weekday: "short" })} ${time}`;
  return t.toLocaleDateString([], { day: "numeric", month: "short", year: t.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

// fmtDate: "7 Oct 2026".
export const fmtDate = (d: When) => toDate(d).toLocaleDateString([], { day: "numeric", month: "short", year: "numeric" });

// fmtDay: "7 Oct" from a YYYY-MM-DD key.
export function fmtDay(iso: string): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y!, m! - 1, d!).toLocaleDateString([], { day: "numeric", month: "short" });
}
