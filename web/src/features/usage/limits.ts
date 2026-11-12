// The agents' plan limits as the page shows them: one subscription is one account whatever machine reads it, so the
// freshest reading of each window wins; a window that has reset says nothing any more. The pure part.

import type { LimitWindow, Machine } from "@/features/roster/api";
import { locale, t } from "@/lib/i18n";

export type LimitTone = "" | "amber" | "red";

// AccountWindow is a window with the machine its reading came from
export type AccountWindow = LimitWindow & { machine: string; machineLabel: string };

const AGENT_ORDER = ["claude", "codex"];
const WINDOW_ORDER = ["5h", "week"];

const rank = (w: LimitWindow) => {
  const agent = AGENT_ORDER.indexOf(w.agent);
  const name = WINDOW_ORDER.indexOf(w.name);
  return (agent < 0 ? AGENT_ORDER.length : agent) * 10 + (name < 0 ? WINDOW_ORDER.length : name);
};

// accountWindows is every agent's windows across the machines: the freshest reading of each, the reset ones left out
export function accountWindows(machines: Machine[], now: number): AccountWindow[] {
  const best = new Map<string, AccountWindow>();
  for (const m of machines) {
    for (const w of m.limits?.windows ?? []) {
      if (Date.parse(w.resets_at) <= now) continue;
      const key = `${w.agent}|${w.name}`;
      const have = best.get(key);
      if (!have || Date.parse(w.at || "") > Date.parse(have.at || "")) best.set(key, { ...w, machine: m.id, machineLabel: m.label });
    }
  }
  return [...best.values()].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name));
}

// agentName is the agent a window belongs to, as a person calls it
export const agentName = (agent: string) => (agent === "claude" ? "Claude" : agent === "codex" ? "Codex" : agent);

// windowName is a window's length in words: "5 h", "week"
export function windowName(name: string): string {
  if (name === "5h") return t("5 h");
  if (name === "week") return t("week");
  return name;
}

// limitTone is the colour a window's use earns: amber from 80 %, red at the limit
export function limitTone(used: number): LimitTone {
  if (used >= 100) return "red";
  if (used >= 80) return "amber";
  return "";
}

// gravest is the tone of the most used window
export const gravest = (windows: LimitWindow[]): LimitTone => limitTone(Math.max(0, ...windows.map((w) => w.used)));

// resetTime is when a window resets: the time of day today, else the weekday and time within a week
export function resetTime(resetsAt: string, now: number): string {
  const at = new Date(resetsAt);
  const time = at.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit" });
  if (at.toDateString() === new Date(now).toDateString()) return time;
  return `${at.toLocaleDateString(locale(), { weekday: "short" })} ${time}`;
}

// limitWords is one window in a line: "Claude 5 h: 62 %, resets 14:00"
export function limitWords(w: LimitWindow, now: number): string {
  return t("{agent} {window}: {used} %, resets {time}", { agent: agentName(w.agent), window: windowName(w.name), used: Math.floor(w.used), time: resetTime(w.resets_at, now) });
}

// FORECAST_AHEAD_MS is how much earlier than its reset a run-out is a concern, the server's push's threshold
export const FORECAST_AHEAD_MS = 30 * 60e3;

// runsOutEarly is how much earlier than its reset the window is forecast to run out, 0 when it is not
export function runsOutEarly(w: LimitWindow): number {
  const out = w.forecast?.runs_out_at;
  return out ? Math.max(0, Date.parse(w.resets_at) - Date.parse(out)) : 0;
}

// forecastWords is the window's forecast in words — "runs out ≈ 15:40 at this pace", "lasts to its reset at this pace" —
// "" when its readings say no pace
export function forecastWords(w: LimitWindow, now: number): string {
  const f = w.forecast;
  if (f?.runs_out_at) return t("runs out ≈ {time} at this pace", { time: resetTime(f.runs_out_at, now) });
  if (f?.lasts) return t("lasts to its reset at this pace");
  return "";
}

// forecastTone is the colour of the forecast: amber for a run-out well before the reset, quiet otherwise
export const forecastTone = (w: LimitWindow): LimitTone => (runsOutEarly(w) >= FORECAST_AHEAD_MS ? "amber" : "");

// limitLine is one window and its forecast in a line, for a tooltip or the machine's ⓘ
export function limitLine(w: LimitWindow, now: number): string {
  const forecast = forecastWords(w, now);
  return forecast ? `${limitWords(w, now)} · ${forecast}` : limitWords(w, now);
}

// resetForHit is the moment an agent stopped on a limit can carry on: once every window of its at the limit has reset —
// the latest of their resets, since a full week still blocks after the 5 hours come back — else the latest of the
// windows at 95 % or more; none when no window says so
export function resetForHit(windows: LimitWindow[], agent: string): string | undefined {
  const own = windows.filter((w) => w.agent === agent);
  for (const floor of [99.5, 95]) {
    const full = own.filter((w) => w.used >= floor).sort((a, b) => Date.parse(b.resets_at) - Date.parse(a.resets_at));
    if (full.length) return full[0]!.resets_at;
  }
  return undefined;
}

// claudeUnread says whether a machine runs Claude Code whose limits nobody reads: no wrapper before its status line
export const claudeUnread = (m: Machine) => !!m.health?.claude && !m.health.statusline && !m.limits?.wrapped;
