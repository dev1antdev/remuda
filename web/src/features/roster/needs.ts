// Who needs the person — a question open, a watchdog note, a finish nobody has looked at — and, for Alt+N, the next such
// agent after the one on screen. Pure functions over the roster, tested beside.

import type { Machine, Pane } from "./api";
import { displayOf, type RadarPrefs } from "./radar";
import { agentKey } from "./sidebarAgents";

export type Needing = { key: string; machine: Machine; pane: Pane };

// needsMe is an agent waiting for the person: a question open, a watchdog note, or a finish nobody has looked at yet
export function needsMe(pane: Pane, radar: RadarPrefs, now: number): boolean {
  if (pane.radar?.alert) return true;
  const d = displayOf(pane, radar, now);
  return d === "blocked" || d === "done";
}

// nextNeedingMe is the agent to go to after the one at fromKey: the agents on connected machines that need the person,
// in the roster's order, round and round; nobody when none needs one, the same agent when it is the only one
export function nextNeedingMe(machines: Machine[], radar: RadarPrefs, now: number, fromKey: string): Needing | null {
  const waiting: Needing[] = [];
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const session of machine.sessions)
      for (const tab of session.tabs)
        for (const pane of tab.panes) if (pane.agent && needsMe(pane, radar, now)) waiting.push({ key: agentKey(machine.id, pane.id), machine, pane });
  }
  if (!waiting.length) return null;
  const at = waiting.findIndex((agent) => agent.key === fromKey);
  return waiting[(at + 1) % waiting.length] ?? null;
}
