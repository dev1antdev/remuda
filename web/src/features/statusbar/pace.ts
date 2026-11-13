// The plan limits as the status bar shows them: each agent's windows in a few characters — "5h 38% · wk 54%" — in the
// tone remuda gives a window's use, and in its card a tick at the share of the window's time gone with the verdict
// on the pace, after Orca's usage limits. What remains may be shown in place of what is used; the colour stays the
// used share's. The pure part.

import type { AccountWindow, LimitTone } from "@/features/usage/limits";
import { limitTone } from "@/features/usage/limits";
import { t } from "@/lib/i18n";

// PACE_MARGIN is how many points the use may run ahead of or behind the time before the verdict says so
export const PACE_MARGIN = 10;

// STALE_MS is how old a reading may be before the card fades it and says how old it is
export const STALE_MS = 15 * 60e3;

const HOUR = 3600e3;

// windowMs is how long a window runs, null for one of no known length
export function windowMs(name: string): number | null {
  if (name === "5h") return 5 * HOUR;
  if (name === "week") return 7 * 24 * HOUR;
  return null;
}

// elapsedShare is how much of the window's time is gone, 0 to 1; null when its length is unknown
export function elapsedShare(w: Pick<AccountWindow, "name" | "resets_at">, now: number): number | null {
  const length = windowMs(w.name);
  const reset = Date.parse(w.resets_at);
  if (!length || !Number.isFinite(reset)) return null;
  return Math.min(1, Math.max(0, 1 - (reset - now) / length));
}

export type Verdict = "ahead" | "on" | "under";

// verdict is how the use stands against the time gone: ahead of it, on it, or behind it by more than the margin
export function verdict(used: number, share: number): Verdict {
  const clock = share * 100;
  if (used > clock + PACE_MARGIN) return "ahead";
  if (used < clock - PACE_MARGIN) return "under";
  return "on";
}

// verdictWords is the verdict as a person says it
export function verdictWords(v: Verdict): string {
  if (v === "ahead") return t("you are using it faster than the time passes");
  if (v === "under") return t("under pace");
  return t("on pace");
}

// tickWords explains the tick in the window's own numbers, the way the setting shows them
export function tickWords(used: number, share: number, remaining: boolean): string {
  const said = verdictWords(verdict(used, share));
  if (remaining) return t("The tick: {time} % of the time is left. You have {left} % left: {verdict}.", { time: Math.round((1 - share) * 100), left: Math.max(0, Math.round(100 - used)), verdict: said });
  return t("The tick: {time} % of the time has passed. You have used {used} %: {verdict}.", { time: Math.round(share * 100), used: Math.floor(used), verdict: said });
}

// timeGone is the tick in words under a window's bar: "40 % of the time gone", or "60 % of the time left"
export const timeGone = (share: number, remaining: boolean): string =>
  remaining ? t("{n} % of the time left", { n: Math.round((1 - share) * 100) }) : t("{n} % of the time gone", { n: Math.round(share * 100) });

// shownShare is the share the bar shows: what is used, or what remains
export const shownShare = (used: number, remaining: boolean): number => Math.min(100, Math.max(0, remaining ? 100 - used : used));

// shortName is a window's name in the bar: 5h, wk
export function shortName(name: string): string {
  if (name === "5h") return t("5h");
  if (name === "week") return t("wk");
  return name;
}

// windowLabel is a window's name in the card: 5-hour, Weekly
export function windowLabel(name: string): string {
  if (name === "5h") return t("5-hour");
  if (name === "week") return t("Weekly");
  return name;
}

// segmentText is one window in the bar: "5h 38%"; what is used is floored, what remains rounded up, so neither
// claims more room than there is
export function segmentText(w: Pick<AccountWindow, "name" | "used">, remaining: boolean): string {
  const shown = remaining ? Math.max(0, Math.ceil(100 - w.used)) : Math.floor(w.used);
  return `${shortName(w.name)} ${shown}%`;
}

// timeLeft is the time to a reset in a few characters: "<1m", "42m", "2h 10m", "3d 4h"
export function timeLeft(resetsAt: string, now: number): string {
  const s = Math.max(0, Math.floor((Date.parse(resetsAt) - now) / 1000));
  if (s < 60) return t("<1m");
  const days = Math.floor(s / 86400);
  const hours = Math.floor((s % 86400) / 3600);
  const minutes = Math.floor((s % 3600) / 60);
  if (days > 0) return t("{d}d {h}h", { d: days, h: hours });
  if (hours > 0) return t("{h}h {m}m", { h: hours, m: minutes });
  return t("{m}m", { m: minutes });
}

// stale says whether a reading is old enough for the card to fade it
export const stale = (at: string, now: number): boolean => {
  const read = Date.parse(at);
  return Number.isFinite(read) && now - read > STALE_MS;
};

// AgentLimits is one agent's windows as the bar groups them, its gravest tone and its newest reading
export type AgentLimits = { agent: string; windows: AccountWindow[]; tone: LimitTone; at: string };

// byAgent groups the account's windows by agent, in the order accountWindows gives them
export function byAgent(windows: AccountWindow[]): AgentLimits[] {
  const out: AgentLimits[] = [];
  for (const w of windows) {
    let group = out.find((g) => g.agent === w.agent);
    if (!group) {
      group = { agent: w.agent, windows: [], tone: "", at: w.at };
      out.push(group);
    }
    group.windows.push(w);
    if (Date.parse(w.at) > Date.parse(group.at)) group.at = w.at;
    const tone = limitTone(w.used);
    if (tone === "red" || (tone === "amber" && group.tone === "")) group.tone = tone;
  }
  return out;
}
