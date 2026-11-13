// The year on the Spend tab: a square a day for the last 365 days, toned by what the day cost or how many responses it
// held, weeks as columns from Monday, the months named over them; and the year's figures — the favourite model, the
// tokens, the sessions and the longest of them, the active days, the streaks, the busiest day. After Orchi's Stats. From
// the server's kept days (UsageYear), the machines the panel leaves out and the folders hidden while the screen is
// shared left out. Pure; Heatmap draws it.

import { locale, t, tn } from "@/lib/i18n";
import { basename, underFolder } from "@/lib/pathname";
import { spent } from "@/lib/time";
import { hidesMachine, type SharingPref } from "@/features/roster/sharing";
import type { UsageYear } from "./api";
import { usd } from "./format";

export type HeatMetric = "cost" | "turns";
export const HEAT_KEY = "usage.heat";
export const LEVELS = 5; // an empty well, then four steps of the accent

export type DayTotal = { date: string; cost: number; tokens: number; turns: number };

export type YearFigures = {
  favourite: string | null; // the model with the most tokens, by its name
  tokens: number;
  sessions: number;
  longest: number; // seconds
  active: number; // days with a response
  span: number; // days in the year shown
  current: number; // the streak of active days up to today, or to yesterday while today has none yet
  best: number;
  busiest: DayTotal | null; // by the metric shown
};

export type YearView = {
  days: Map<string, DayTotal>;
  hidden: number; // folders left out while the screen is shared
  reading: number; // the most ended days still being read on a machine shown
  away: string[]; // machines not connected, drawn as last read
  failed: { label: string; error: string }[];
  readAt: string | null; // the oldest of the machines' readings of today, RFC 3339
};

// Hides says whether a folder of a machine is left out of the year
export type Hides = (machineId: string, dir: string) => boolean;

const WORKTREE = /[\\/]\.herdr[\\/]worktrees[\\/]([^\\/]+)[\\/]/;

// hiddenFolder says whether the screen-sharing mode hides a folder the year names: its machine, every folder, or a
// folder marked — the folder itself, one it lies in, a workspace of the roster on it whose folder is marked, or a herdr
// worktree of a marked repository
export function hiddenFolder(pref: SharingPref, sessions: { dir: string; folder: string }[], machineId: string, dir: string): boolean {
  if (!pref.on) return false;
  if (pref.all || hidesMachine(pref, machineId)) return true;
  const marked = pref.folders.filter((key) => key.startsWith(`${machineId}|`)).map((key) => key.slice(machineId.length + 1));
  if (marked.length === 0) return false;
  const within = (folder: string) => dir === folder || underFolder(dir, folder) !== null;
  if (marked.some(within)) return true;
  const repo = WORKTREE.exec(dir + "/")?.[1];
  if (repo && marked.some((folder) => basename(folder) === repo)) return true;
  return sessions.some((s) => s.dir && (dir === s.dir || underFolder(dir, s.dir) !== null) && marked.includes(s.folder));
}

// yearView adds the machines shown up day by day — "all" or one machine's id — the busiest day by the metric shown
export function yearView(year: UsageYear, machine: string, hides: Hides, metric: HeatMetric): YearView & { figures: YearFigures } {
  const days = new Map<string, DayTotal>();
  const models = new Map<string, number>();
  let sessions = 0;
  let longest = 0;
  let hidden = 0;
  let reading = 0;
  const away: string[] = [];
  const failed: { label: string; error: string }[] = [];
  let readAt: string | null = null;
  for (const m of year.machines) {
    if (machine !== "all" && m.id !== machine) continue;
    reading = Math.max(reading, m.reading ?? 0);
    if (m.away) away.push(m.label);
    if (m.error) failed.push({ label: m.label, error: m.error });
    if (m.today_at && (!readAt || m.today_at < readAt)) readAt = m.today_at;
    const shown = m.folders.map((f) => !hides(m.id, f.dir ?? ""));
    m.folders.forEach((f, i) => {
      if (!shown[i]) {
        hidden++;
        return;
      }
      sessions += f.sessions;
      longest = Math.max(longest, f.longest);
      for (const [model, n] of Object.entries(f.models ?? {})) models.set(model, (models.get(model) ?? 0) + n);
    });
    for (const day of m.days) {
      for (const cell of day.cells) {
        if (!shown[cell.f]) continue;
        const kept = days.get(day.date) ?? { date: day.date, cost: 0, tokens: 0, turns: 0 };
        kept.cost += cell.c;
        kept.tokens += cell.t;
        kept.turns += cell.n;
        days.set(day.date, kept);
      }
    }
  }
  let favourite: string | null = null;
  let most = 0;
  for (const [model, n] of models) {
    if (n > most) {
      most = n;
      favourite = year.names[model] ?? model;
    }
  }
  let tokens = 0;
  for (const day of days.values()) tokens += day.tokens;
  const dates = datesOf(year.from, year.to);
  const active = new Set(dates.filter((d) => (days.get(d)?.turns ?? 0) > 0));
  return { days, hidden, reading, away, failed, readAt, figures: { favourite, tokens, sessions, longest, active: active.size, span: dates.length, ...streaks(dates, active), busiest: busiestDay(days, metric) } };
}

// busiestDay is the day the metric puts first, null in a year with nothing in it
export function busiestDay(days: Map<string, DayTotal>, metric: HeatMetric): DayTotal | null {
  let best: DayTotal | null = null;
  for (const day of days.values()) if (valueOf(day, metric) > 0 && (!best || valueOf(day, metric) > valueOf(best, metric))) best = day;
  return best;
}

export const valueOf = (day: DayTotal | undefined, metric: HeatMetric): number => (day ? (metric === "cost" ? day.cost : day.turns) : 0);

const DAY_MS = 86_400_000;
const utc = (date: string) => Date.UTC(+date.slice(0, 4), +date.slice(5, 7) - 1, +date.slice(8, 10));
const iso = (ms: number) => new Date(ms).toISOString().slice(0, 10);

// datesOf is every date from one to the other, both kept
export function datesOf(from: string, to: string): string[] {
  const out: string[] = [];
  for (let at = utc(from); at <= utc(to); at += DAY_MS) out.push(iso(at));
  return out;
}

// streaks are the runs of active days: the longest, and the one that reaches today — or yesterday, while today has had
// nothing yet, so a morning breaks no streak
export function streaks(dates: string[], active: Set<string>): { current: number; best: number } {
  let best = 0;
  let run = 0;
  for (const date of dates) {
    run = active.has(date) ? run + 1 : 0;
    best = Math.max(best, run);
  }
  let i = dates.length - 1;
  if (i >= 0 && !active.has(dates[i]!)) i--;
  let current = 0;
  for (; i >= 0 && active.has(dates[i]!); i--) current++;
  return { current, best };
}

export type HeatCell = { date: string; col: number; row: number; level: number; value: number };
export type HeatGrid = { cells: HeatCell[]; weeks: number; months: { col: number; date: string }[] };

// rowOf is a date's row, Monday first
const rowOf = (date: string) => (new Date(utc(date)).getUTCDay() + 6) % 7;

// heatGrid lays the year out a column a week, Monday at the top: each day its level — 0 for nothing, then the quartile of
// the year's busy days it falls in — and the months named at the column where each begins, two closer than three
// columns keeping the later
export function heatGrid(from: string, to: string, value: (date: string) => number): HeatGrid {
  const dates = datesOf(from, to);
  const lead = rowOf(from);
  const values = dates.map(value);
  const busy = values.filter((v) => v > 0).sort((a, b) => a - b);
  const at = (q: number) => busy[Math.min(busy.length - 1, Math.floor(q * busy.length))] ?? 0;
  const [q1, q2, q3] = [at(0.25), at(0.5), at(0.75)];
  const level = (v: number) => (v <= 0 ? 0 : v <= q1 ? 1 : v <= q2 ? 2 : v <= q3 ? 3 : 4);
  const cells = dates.map((date, i) => ({ date, col: Math.floor((i + lead) / 7), row: (i + lead) % 7, level: level(values[i]!), value: values[i]! }));
  const months: { col: number; date: string }[] = [];
  for (const cell of cells) {
    const month = cell.date.slice(0, 7);
    if (months.length === 0 || months[months.length - 1]!.date.slice(0, 7) !== month) months.push({ col: cell.col, date: cell.date });
  }
  const spaced = months.filter((m, i) => i === months.length - 1 || months[i + 1]!.col - m.col >= 3);
  return { cells, weeks: (cells[cells.length - 1]?.col ?? 0) + 1, months: spaced };
}

// cellAt is the day at a column and row of the grid, if the year has one there
export function cellAt(grid: HeatGrid, col: number, row: number): HeatCell | null {
  const first = grid.cells[0];
  if (!first) return null;
  const i = col * 7 + row - first.row;
  const cell = grid.cells[i];
  return cell && cell.col === col && cell.row === row ? cell : null;
}

const dayFormat = () => new Intl.DateTimeFormat(locale(), { weekday: "short", day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });

// monthWords is a month's short name, weekdayWords a weekday's (0 Monday), as the language writes them
export const monthWords = (date: string) => new Intl.DateTimeFormat(locale(), { month: "short", timeZone: "UTC" }).format(utc(date));
export const weekdayWords = (row: number) => new Intl.DateTimeFormat(locale(), { weekday: "short", timeZone: "UTC" }).format(Date.UTC(2024, 0, 1 + row));

// dayWords is a square's line: "Tue, 12 Nov 2026 · $4.20 · 312 responses", or that nothing was spent
export function dayWords(date: string, day: DayTotal | undefined): string {
  const when = dayFormat().format(utc(date));
  if (!day || day.turns === 0) return t("{day} · nothing", { day: when });
  return `${when} · ${usd(day.cost)} · ${tn(day.turns, "{n} response", "{n} responses")}`;
}

// lengthWords is a session's length: "40 s", "9 min", "5 h 12 min", "4 d 23 h"
export function lengthWords(seconds: number): string {
  if (seconds < 86_400) return spent(seconds);
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3600);
  return hours ? t("{d} d {h} h", { d: days, h: hours }) : tn(days, "{n} day", "{n} days");
}

// readingWords is what the year waits for: "Reading 365 days…"
export const readingWords = (n: number) => tn(n, "Reading {n} day…", "Reading {n} days…");
