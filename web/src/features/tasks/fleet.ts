// A project's queue free to use any machine: where its next card goes and why, why the other machines have no room —
// the words of the line under the Queued column. The server weighs and picks (internal/placement); this is the words. Pure.

import { agentName } from "@/features/usage/limits";
import { t, tn } from "@/lib/i18n";
import type { Fleet, FleetMachine } from "./api";

const perCore = (m: FleetMachine) => t("load {load} per core", { load: m.load_per_core.toFixed(1) });

// factsOf is a machine's figures as the line says them: "load 0.3 per core, 1 agent working, Claude at 40 %"
export function factsOf(m: FleetMachine, agent: string): string {
  const facts = [perCore(m)];
  if (m.temp_c && m.temp_c >= 75) facts.push(t("warm at {temp} °C", { temp: m.temp_c.toFixed(0) }));
  if (m.battery?.discharging) facts.push(t("on battery at {pct} %", { pct: m.battery.percent }));
  facts.push(m.working === 0 ? t("no agent working") : tn(m.working, "{n} agent working", "{n} agents working"));
  if (m.plan >= 0) facts.push(t("{agent} at {pct} %", { agent: agentName(agent), pct: Math.round(m.plan) }));
  return facts.join(", ");
}

// outWords is why a machine has no room: "GOG: hot at 88 °C"; a plan or a budget in the server's words
export function outWords(m: FleetMachine): string {
  switch (m.out) {
    case "hot":
      return `${m.label}: ${t("hot at {temp} °C", { temp: (m.temp_c ?? 0).toFixed(0) })}`;
    case "battery":
      return `${m.label}: ${t("on battery at {pct} %", { pct: m.battery?.percent ?? 0 })}`;
    case "load":
      return `${m.label}: ${perCore(m)}`;
    case "plan":
      return `${m.label}: ${m.held ?? ""}`;
    case "budget":
      return `${m.label}: ${m.budget ?? ""}`;
  }
  return m.label;
}

// nextWords is where the next card would go and why, "" with no machine picked
export const nextWords = (fleet: Fleet | undefined, agent: string): string => (fleet?.pick ? t("next on {machine}: {facts}", { machine: fleet.pick.label, facts: factsOf(fleet.pick, agent) }) : "");

// noRoomWords is why no machine takes the card: each one's reason, or that none holds a checkout of the repository
export function noRoomWords(fleet: Fleet): string {
  const out = (fleet.others ?? []).filter((m) => m.out);
  if (!out.length) return t("no connected machine has a workspace open on a checkout of this repository");
  return t("no machine has room: {why}", { why: out.map(outWords).join("; ") });
}
