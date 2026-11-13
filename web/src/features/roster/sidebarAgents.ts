// The sidebar's agents, after Orca and munza's better-sidebar: each agent's state as a shape that reads without colour,
// the "Needs you" inbox of every agent waiting on the person, the counts a folded head and the footer say, the pins,
// the subagents under their agent, and the order Alt+] and Alt+[ walk. Pure functions over the roster, tested beside.

import { t, tn } from "@/lib/i18n";
import type { Machine, Pane, Session, Tab, WorkingSubagent } from "./api";
import type { AgentRow, Display } from "./radar";
import { entries, entryItems } from "./workspaces";
import type { RadarPrefs } from "./radar";

// Shape is how an agent's line marks its state: working the spinner, asking a "Needs you" pill, a watchdog's note a "!"
// square, done a filled dot, anything else a hollow ring with the name muted
export type Shape = "working" | "asking" | "note" | "done" | "idle";

export function shapeOf(p: Pane, d: Display): Shape {
  if (p.radar?.state === "blocked") return "asking";
  if (p.radar?.alert) return "note";
  if (d === "working") return "working";
  if (d === "done") return "done";
  return "idle";
}

// needsYou says whether an agent waits on the person: a question open, or a watchdog's note on it
export const needsYou = (p: Pane): boolean => !!p.agent && (p.radar?.state === "blocked" || !!p.radar?.alert);

// Waiting is one row of the inbox: the agent, where it is, since when it waits and whether it asks or carries a note
export type Waiting = { key: string; machine: Machine; session: Session; tab: Tab; pane: Pane; since?: number };

// agentKey names an agent across machines, the way the pins and the walk keep it
export const agentKey = (machineId: string, paneId: string): string => `${machineId}|${paneId}`;

// waitingAgents is every agent on a connected machine that waits on the person, the oldest wait first (one with no
// stamp last), ties in the roster's order
export function waitingAgents(machines: Machine[]): Waiting[] {
  const out: (Waiting & { index: number })[] = [];
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const session of machine.sessions) {
      if (session.scratch || session.concierge) continue;
      for (const tab of session.tabs)
        for (const pane of tab.panes) {
          if (!needsYou(pane)) continue;
          out.push({ key: agentKey(machine.id, pane.id), machine, session, tab, pane, since: pane.radar?.since || pane.radar?.last_active || undefined, index: out.length });
        }
    }
  }
  out.sort((a, b) => (a.since ?? Infinity) - (b.since ?? Infinity) || a.index - b.index);
  return out.map(({ index: _, ...w }) => w);
}

// Tally is what a folded head or the footer says of a set of workspaces: how many agents, how many wait on you, how
// many work (a stuck one counts as waiting, not as working), how many finished
export type Tally = { agents: number; needs: number; working: number; done: number };

export function tallyOf(sessions: Session[]): Tally {
  const n: Tally = { agents: 0, needs: 0, working: 0, done: 0 };
  for (const s of sessions) {
    if (s.scratch || s.concierge) continue;
    for (const tab of s.tabs)
      for (const p of tab.panes) {
        if (!p.agent) continue;
        n.agents++;
        if (needsYou(p)) n.needs++;
        else if (p.radar?.state === "working") n.working++;
        else if (p.radar?.state === "done") n.done++;
      }
  }
  return n;
}

// fleetTally is the footer's tally: every agent on the connected machines
export const fleetTally = (machines: Machine[]): Tally => tallyOf(machines.filter((m) => m.state === "connected").flatMap((m) => m.sessions));

// needWords is the chip of a folded head and the footer's link: "2 need you"
export const needWords = (n: number): string => tn(n, "{n} needs you", "{n} need you");

// workingWords is the words beside a folded head's spinner: "1 working"
export const workingWords = (n: number): string => tn(n, "{n} working", "{n} working");

// agentsWords is the footer's count: "3 agents"
export const agentsWords = (n: number): string => tn(n, "{n} agent", "{n} agents");

// ---- pins ----

export const PINS_KEY = "sidebar.pins";

// togglePin pins an agent at the end of the pinned group, or unpins it
export const togglePin = (pins: string[], key: string): string[] => (pins.includes(key) ? pins.filter((p) => p !== key) : [...pins, key]);

// Pinned is a pinned agent's line: its row, and the machine it runs on
export type Pinned = { key: string; machine: Machine; row: AgentRow };

// pinnedRows is the pinned group in pin order: every pinned agent the roster shows on a connected machine
export function pinnedRows(machines: Machine[], pins: string[]): Pinned[] {
  const found = new Map<string, Pinned>();
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const session of machine.sessions) {
      if (session.scratch || session.concierge) continue;
      session.tabs.forEach((tab) =>
        tab.panes.forEach((pane, i) => {
          const key = agentKey(machine.id, pane.id);
          if (pane.agent && pins.includes(key)) found.set(key, { key, machine, row: { session, tab, pane, last: pane.radar?.last_active ?? 0, head: i === 0, index: i } });
        }),
      );
    }
  }
  return pins.flatMap((key) => found.get(key) ?? []);
}

// livePins drops the pins of agents gone: their machine is connected, shows its workspaces and has no such agent, or the
// fleet no longer has the machine; a machine away, still connecting or not read yet keeps its pins, since its agents may
// well be there. Herdr gives a closed pane's id to a new one later, which a stale pin would otherwise take for the old agent.
export function livePins(machines: Machine[], pins: string[]): string[] {
  if (machines.length === 0) return pins;
  const byId = new Map(machines.map((m) => [m.id, m]));
  return pins.filter((key) => {
    const at = key.indexOf("|");
    const machine = byId.get(key.slice(0, at));
    if (!machine) return false;
    if (machine.state !== "connected" || machine.sessions.length === 0) return true;
    const pane = key.slice(at + 1);
    return machine.sessions.some((s) => s.tabs.some((tab) => tab.panes.some((p) => p.id === pane && !!p.agent)));
  });
}

// ---- subagents ----

// subagentsOf is the subagents at work under an agent's line, the oldest first, and how many more work unnamed
export function subagentsOf(p: Pane): { list: WorkingSubagent[]; more: number } {
  const list = p.subagents?.list ?? [];
  return { list, more: Math.max(0, (p.subagents?.working ?? 0) - list.length) };
}

// subagentCount is how many work under an agent: what its folded line counts
export const subagentCount = (p: Pane): number => Math.max(p.subagents?.working ?? 0, p.subagents?.list?.length ?? 0);

// subagentsWord is a folded line's tooltip: "2 subagents working"
export const subagentsWord = (n: number): string => tn(n, "{n} subagent working", "{n} subagents working");

// elbowOf is the tree line in front of a subagent: an elbow that goes on for every one but the last
export const elbowOf = (index: number, count: number): "├" | "└" => (index === count - 1 ? "└" : "├");

// ---- the walk ----

// walkOrder is every agent in the order the sidebar draws them — the pinned ones first, then each connected machine's
// workspaces as its order lays them out, an agent's subagents' parent among them — each once
export function walkOrder(machines: Machine[], prefs: RadarPrefs, pins: string[]): { machineId: string; paneId: string }[] {
  const seen = new Set<string>();
  const out: { machineId: string; paneId: string }[] = [];
  const add = (machineId: string, paneId: string) => {
    const key = agentKey(machineId, paneId);
    if (seen.has(key)) return;
    seen.add(key);
    out.push({ machineId, paneId });
  };
  for (const pinned of pinnedRows(machines, pins)) add(pinned.machine.id, pinned.row.pane.id);
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const entry of entries(machine, prefs)) for (const item of entryItems(entry)) for (const r of item.rows) if (r.pane.agent) add(machine.id, r.pane.id);
  }
  return out;
}

// stepAgent is the agent after the one on screen (or before it, step -1), round and round; the first (or the last)
// when the one on screen is not among them; null when there is none
export function stepAgent(order: { machineId: string; paneId: string }[], from: string, step: 1 | -1): { machineId: string; paneId: string } | null {
  if (order.length === 0) return null;
  const at = order.findIndex((a) => agentKey(a.machineId, a.paneId) === from);
  if (at < 0) return step === 1 ? order[0]! : order[order.length - 1]!;
  return order[(at + step + order.length) % order.length]!;
}

// ---- words ----

// askWords is what an inbox row says the agent waits on: what it asks, read off its screen, else the watchdog's note,
// else that it waits for an answer
export function askWords(p: Pane, asked: string | undefined): string {
  if (p.radar?.state === "blocked") return asked || t("waiting for your answer");
  return p.radar?.alert || t("waiting for your answer");
}

// syncWords is the selected card's distance from its upstream, "↑2 ↓1", the zeros left out; "" when level or unknown
export function syncWords(sync: Session["sync"]): string {
  if (!sync) return "";
  return [sync.ahead > 0 ? `↑${sync.ahead}` : "", sync.behind > 0 ? `↓${sync.behind}` : ""].filter(Boolean).join(" ");
}

// syncTip is that distance in words, for its tooltip
export function syncTip(sync: Session["sync"]): string {
  if (!sync) return "";
  return [
    sync.ahead > 0 ? tn(sync.ahead, "{n} commit not pushed", "{n} commits not pushed") : "",
    sync.behind > 0 ? tn(sync.behind, "{n} commit to pull", "{n} commits to pull") : "",
    t("as of the last fetch"),
  ]
    .filter(Boolean)
    .join(" · ");
}
