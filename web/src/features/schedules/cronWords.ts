// A cron in words, any cron: which days and at what times it runs — "Mon–Fri, every 10 min, 9:00–16:50" — for the form
// and the list; null for an expression it cannot read, which the server refuses with its reason. Read as the server's
// own parser reads it (internal/schedule): Vixie's five fields, day-of-week 7 a Sunday too, the two day fields of a
// restricted pair matching either one.

import { locale, t } from "@/lib/i18n";

type Fields = {
  minutes: number[];
  hours: number[];
  doms: number[] | null; // null: every day of the month
  months: number[] | null;
  weekdays: number[] | null; // 0 Sunday … 6 Saturday
  eitherDay: boolean; // both day fields restricted: a day matching either one runs
};

const RANGES: [number, number][] = [
  [0, 59],
  [0, 23],
  [1, 31],
  [1, 12],
  [0, 7],
];

// values reads one field into its sorted values, null when it does not parse
function values(field: string, [lo, hi]: [number, number]): number[] | null {
  const allowed = new Set<number>();
  for (const part of field.split(",")) {
    const [range, stepText] = part.split("/");
    const step = stepText === undefined ? 1 : Number(stepText);
    if (!Number.isInteger(step) || step < 1 || range === undefined || range === "") return null;
    let a = lo;
    let b = hi;
    if (range !== "*") {
      const [x, y] = range.split("-");
      a = Number(x);
      b = y === undefined ? (stepText === undefined ? a : hi) : Number(y);
      if (!/^\d+$/.test(x ?? "") || (y !== undefined && !/^\d+$/.test(y))) return null;
    }
    if (a < lo || b > hi || a > b) return null;
    for (let v = a; v <= b; v += step) allowed.add(v);
  }
  return [...allowed].sort((x, y) => x - y);
}

// parseFields reads a cron's five fields, null when one does not parse
export function parseFields(cron: string): Fields | null {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return null;
  const read = parts.map((p, i) => values(p, RANGES[i]!));
  if (read.some((v) => v === null)) return null;
  const [minutes, hours, doms, months, weekdayValues] = read as number[][];
  const weekdays = [...new Set(weekdayValues!.map((d) => d % 7))].sort((x, y) => x - y);
  const anyDom = parts[2]!.startsWith("*");
  const anyWeekday = parts[4]!.startsWith("*");
  return {
    minutes: minutes!,
    hours: hours!,
    doms: doms!.length === 31 ? null : doms!,
    months: months!.length === 12 ? null : months!,
    weekdays: weekdays.length === 7 ? null : weekdays,
    eitherDay: !anyDom && !anyWeekday,
  };
}

const pad = (n: number) => String(n).padStart(2, "0");
const clock = (h: number, m: number) => `${h}:${pad(m)}`;

// stepOf is the step of values that walk a whole cycle of `size` evenly from the first, 0 for any other
function stepOf(list: number[], size: number): number {
  if (list.length < 2) return 0;
  const step = list[1]! - list[0]!;
  if (list.some((v, i) => i > 0 && v - list[i - 1]! !== step)) return 0;
  return list[0]! < step && list[list.length - 1]! + step >= size ? step : 0;
}

// unbroken says whether sorted values run without a gap
const unbroken = (list: number[]) => list[list.length - 1]! - list[0]! === list.length - 1;

// compact lists values with runs of three or more as a range: "1–5, 15"
function compact(list: number[]): string {
  const out: string[] = [];
  for (let i = 0; i < list.length; ) {
    let j = i;
    while (j + 1 < list.length && list[j + 1] === list[j]! + 1) j++;
    out.push(j - i >= 2 ? `${list[i]}–${list[j]}` : list.slice(i, j + 1).join(", "));
    i = j + 1;
  }
  return out.join(", ");
}

const weekdayName = (day: number) => new Intl.DateTimeFormat(locale(), { weekday: "short" }).format(new Date(2024, 0, 7 + day)); // 7 Jan 2024 was a Sunday
const monthName = (month: number) => new Intl.DateTimeFormat(locale(), { month: "short" }).format(new Date(2024, month - 1, 1));

// weekdayWords names the days of the week, Monday first: "Mon–Fri", "weekends", "Mon, Wed, Fri"
function weekdayWords(days: number[]): string {
  const mondayFirst = [1, 2, 3, 4, 5, 6, 0].filter((d) => days.includes(d));
  const key = mondayFirst.join("");
  if (key === "12345") return t("Mon–Fri");
  if (key === "60") return t("weekends");
  const places = mondayFirst.map((d) => (d + 6) % 7);
  if (mondayFirst.length >= 3 && unbroken(places)) return `${weekdayName(mondayFirst[0]!)}–${weekdayName(mondayFirst[mondayFirst.length - 1]!)}`;
  return mondayFirst.map(weekdayName).join(", ");
}

// dayWords says on which days it runs
function dayWords(f: Fields): string {
  const days: string[] = [];
  if (f.weekdays) days.push(weekdayWords(f.weekdays));
  if (f.doms) days.push(t("on day {days} of the month", { days: compact(f.doms) }));
  let words = days.length === 0 ? t("every day") : days.join(f.eitherDay ? t(" or ") : ", ");
  if (f.months) words += `, ${t("in {months}", { months: f.months.map(monthName).join(", ") })}`;
  return words;
}

// timeWords says at what times of those days it runs
function timeWords({ minutes, hours }: Fields): string {
  const allHours = hours.length === 24;
  const first = hours[0]!;
  const last = hours[hours.length - 1]!;
  if (minutes.length === 1) {
    const m = minutes[0]!;
    if (allHours) return t("every hour at :{mm}", { mm: pad(m) });
    if (hours.length === 1) return t("at {time}", { time: clock(first, m) });
    const hourStep = stepOf(hours, 24);
    if (hourStep > 1) return first === 0 ? t("every {n} h at :{mm}", { n: hourStep, mm: pad(m) }) : t("every {n} h from {time}", { n: hourStep, time: clock(first, m) });
    if (unbroken(hours)) return `${t("every hour at :{mm}", { mm: pad(m) })}, ${clock(first, m)}–${clock(last, m)}`;
    return t("at {times}", { times: hours.map((h) => clock(h, m)).join(", ") });
  }
  const minuteStep = stepOf(minutes, 60);
  const often =
    minutes.length === 60
      ? t("every minute")
      : minuteStep
        ? minutes[0] === 0
          ? t("every {n} min", { n: minuteStep })
          : t("every {n} min from :{mm}", { n: minuteStep, mm: pad(minutes[0]!) })
        : t("at minutes {list}", { list: minutes.map(pad).join(", ") });
  if (allHours) return often;
  if (unbroken(hours)) return `${often}, ${clock(first, minutes[0]!)}–${clock(last, minutes[minutes.length - 1]!)}`;
  return `${often}, ${t("in the hours {hours}", { hours: compact(hours) })}`;
}

// cronWords is the cron in words, its first letter capitalised; null when it does not parse
export function cronWords(cron: string): string | null {
  const fields = parseFields(cron);
  if (!fields) return null;
  const words = `${dayWords(fields)}, ${timeWords(fields)}`;
  return words.charAt(0).toUpperCase() + words.slice(1);
}

// offsetOf reads the UTC offset an RFC 3339 time carries, in minutes; null when it carries none
export function offsetOf(iso: string): number | null {
  if (/Z$/i.test(iso)) return 0;
  const m = /([+-])(\d{2}):(\d{2})$/.exec(iso);
  return m ? (m[1] === "-" ? -1 : 1) * (Number(m[2]) * 60 + Number(m[3])) : null;
}

// hostClockWords says, when the host's clock is another zone than this device's at that moment, which one the cron reads; "" when they agree
export function hostClockWords(iso: string, deviceOffset: number): string {
  const host = offsetOf(iso);
  if (host === null || host === deviceOffset) return "";
  const sign = host < 0 ? "−" : "+";
  const abs = Math.abs(host);
  return t("on the host's clock, UTC{offset}", { offset: `${sign}${Math.floor(abs / 60)}${abs % 60 ? `:${pad(abs % 60)}` : ""}` });
}
