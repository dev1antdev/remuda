// A workspace put to sleep, after Orca's hibernate mod: which agents a sleep would interrupt, the words of its confirm
// and its toasts, the sleeping card's tip, and the key that wakes a workspace once when it is opened. Pure; the flows
// with their toasts are flows.ts.

import { t, tn } from "@/lib/i18n";
import { VENDOR_NAME } from "@/ui/brand";
import type { Session } from "@/features/roster/api";
import type { Busy } from "./api";

const agentName = (agent: string): string => VENDOR_NAME[agent] ?? agent;

// agentPanes are the workspace's panes that run an agent
const agentPanes = (s: Session) => s.tabs.flatMap((tab) => tab.panes.filter((p) => p.agent));

// canSleep says whether a workspace has anything to put to sleep: an agent in one of its panes
export const canSleep = (s: Session): boolean => agentPanes(s).length > 0;

// busyOf is the agents a sleep would interrupt, as the roster reads them: at work, or asking the person something
export function busyOf(s: Session): Busy[] {
  const out: Busy[] = [];
  for (const p of agentPanes(s)) {
    const state = p.radar?.state ?? (p.status === "run" ? "working" : p.status === "input" ? "blocked" : "");
    if (state === "working" || state === "blocked") out.push({ pane: p.id, agent: p.agent, status: state });
  }
  return out;
}

// busyWords is one busy agent in the confirm: "Claude Code is working"
export const busyWords = (b: Busy): string => (b.status === "blocked" ? t("{agent} is waiting for your answer", { agent: agentName(b.agent) }) : t("{agent} is working", { agent: agentName(b.agent) }));

// sleepAsk is the confirm before a sleep that interrupts someone, Orca's words
export const sleepAsk = (name: string, busy: Busy[]): string =>
  t("Sleep {name}? Still busy: {busy}. Sleeping interrupts them. Waking resumes their conversations where they stopped.", { name, busy: busy.map(busyWords).join(", ") });

// sleptWords is the toast once the sleep is under way: "api is asleep · 3 agents stopped. Wake it from its menu."
export const sleptWords = (name: string, n: number): string => tn(n, "{name} is asleep · {n} agent stopped. Wake it from its menu.", "{name} is asleep · {n} agents stopped. Wake it from its menu.", { name });

// wokeWords is the toast once a wake brought every agent back
export const wokeWords = (name: string, n: number): string => tn(n, "{name} is awake · {n} agent resumed", "{name} is awake · {n} agents resumed", { name });

// asleepTip is the sleeping card's tip: "Asleep · 3 agents stopped · Wake from the menu"
export const asleepTip = (n: number): string => tn(n, "Asleep · {n} agent stopped · Wake from the menu", "Asleep · {n} agents stopped · Wake from the menu");

// wakeKey names one sleep of one workspace: a wake on opening it is asked once per sleep, never again for the same
export const wakeKey = (machine: string, s: Session): string => (s.asleep ? `${machine}|${s.id}|${s.asleep.at}` : "");
