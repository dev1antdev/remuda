// The plan limits' offers as the page shows them: a queued card or an agent schedule whose vendor's window is nearly
// spent waits, and the person may run that very one now on the other vendor's agent — the toast's button, a row of the
// Usage panel, the push's. The server decides and runs (internal/limits offers.go); this is the words. Pure.

import { t } from "@/lib/i18n";
import { agentName, resetTime, windowName } from "./limits";

// PlanOffer is one open offer as the server keeps it; why is the hold in the server's words
export type PlanOffer = {
  id: string;
  kind: "task" | "schedule";
  project?: number;
  task?: number;
  ref?: string; // "#12"
  schedule?: string;
  title: string;
  machine?: string; // "" for a project whose queue picks the machine
  from: string;
  to: string;
  model?: string; // a recipe's model for the other agent, else its default
  why: string;
  window?: string; // the held window: 5h | week
  used?: number; // its use when the offer was made
  runs_out_at?: string; // when it runs out at this pace, for a window held on its forecast
  resets_at: string;
  at: string;
};

// holdWords is why the work waits, in the page's words — the held window, how full or when it runs out, its reset —
// else the server's own words
export function holdWords(o: PlanOffer, now: number): string {
  if (!o.window) return o.why;
  const vars = { agent: agentName(o.from), window: windowName(o.window), used: Math.floor(o.used ?? 0), time: resetTime(o.resets_at, now) };
  if (o.runs_out_at) return t("{agent} {window} runs out ≈ {at} at this pace, before its reset at {time}", { ...vars, at: resetTime(o.runs_out_at, now) });
  if ((o.used ?? 0) >= 100) return t("{agent} {window} is spent, resets {time}", vars);
  return t("{agent} {window} is at {used} %, resets {time}", vars);
}

// MAX_SEEN is how many offers this device remembers having shown
export const MAX_SEEN = 30;

// agentLabel is an agent kind as the offers name it
export const agentLabel = (kind: string): string => (kind === "claude" ? "Claude Code" : kind === "codex" ? "Codex" : kind);

// whatWaits is the held work in words: "Task #12 “Fix the login page”", "Schedule “Look around”"
export const whatWaits = (o: PlanOffer): string => (o.kind === "task" ? t("Task {ref} “{title}”", { ref: o.ref ?? "", title: o.title }) : t("Schedule “{title}”", { title: o.title }));

// offerWords is the toast's line: what waits on whose plan limit
export const offerWords = (o: PlanOffer): string => t("{what} waits for {agent}'s plan limit.", { what: whatWaits(o), agent: agentLabel(o.from) });

// runLabel is the button that runs it on the other vendor
export const runLabel = (o: PlanOffer): string => t("Run on {agent}", { agent: agentLabel(o.to) });

// modelWords says which model the other agent runs on: a recipe's, or its default
export const modelWords = (o: PlanOffer): string => (o.model ? t("on {model}, as your recipe says", { model: o.model }) : t("on its default model"));

// offersToShow are the open offers this device has not shown yet, oldest first
export const offersToShow = (offers: PlanOffer[], seen: string[]): PlanOffer[] => offers.filter((o) => !seen.includes(o.id));

// rememberOffer notes an offer shown here, the newest MAX_SEEN kept
export function rememberOffer(seen: string[], id: string): string[] {
  return [...seen.filter((s) => s !== id), id].slice(-MAX_SEEN);
}
