// The one set of time words: how long ago, when (short), a date.

type When = Date | string | number;
const toDate = (d: When) => (d instanceof Date ? d : new Date(d));

// "just now", "5 min ago", "3 h ago", "2 days ago"
export function ago(d: When, now = Date.now()): string {
  const m = Math.round((now - toDate(d).getTime()) / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}

// the time if today, the weekday and time within a week, else the date
export function whenShort(d: When): string {
  const t = toDate(d);
  const now = new Date();
  const time = t.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  if (t.toDateString() === now.toDateString()) return time;
  const days = Math.round((now.getTime() - t.getTime()) / 86400000);
  if (days >= 0 && days < 7) return `${t.toLocaleDateString([], { weekday: "short" })} ${time}`;
  return t.toLocaleDateString([], { day: "numeric", month: "short", year: t.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

// "7 Oct 2026"
export const fmtDate = (d: When) => toDate(d).toLocaleDateString([], { day: "numeric", month: "short", year: "numeric" });

// "7 Oct" from a YYYY-MM-DD key
export function fmtDay(iso: string): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y!, m! - 1, d!).toLocaleDateString([], { day: "numeric", month: "short" });
}
