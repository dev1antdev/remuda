// The arena's pure part: the lanes a new arena starts with, why one cannot start yet, and what a lane's card says.

import type { AgentKind, Machine, Pane } from "@/features/roster/api";
import { CUSTOM } from "@/features/roster/startOptions";
import { t, tn } from "@/lib/i18n";
import type { Lane, LaneRequest, Stat } from "./api";

export const MIN_LANES = 2;
export const MAX_LANES = 6;

// LaneDraft is a lane as the dialog edits it: the model a choice of the known names, or a typed id behind CUSTOM
export type LaneDraft = { machine: string; workspace: string; agent: string; choice: string; custom: string };

// repositories are a machine's workspaces a worktree can branch off: the repositories themselves, not their worktrees
export function repositories(machines: Machine[], machineId: string): { value: string; label: string; hint: string }[] {
  const machine = machines.find((m) => m.id === machineId);
  return (machine?.sessions ?? []).filter((s) => s.dir && !s.worktree?.linked).map((s) => ({ value: s.id, label: s.name, hint: s.dir ?? "" }));
}

// defaultLanes are two lanes on one repository — the one on screen when it is one, else the first connected machine's
// first — the second agent another kind when there is one, so the pair compares two agents
export function defaultLanes(machines: Machine[], agents: AgentKind[], at?: { machineId: string; workspaceId: string }): LaneDraft[] {
  const ready = machines.filter((m) => m.state === "connected" && repositories(machines, m.id).length > 0);
  const here = at && ready.some((m) => m.id === at.machineId) && repositories(machines, at.machineId).some((r) => r.value === at.workspaceId) ? at : undefined;
  const machine = here?.machineId ?? ready[0]?.id ?? "";
  const workspace = here?.workspaceId ?? repositories(machines, machine)[0]?.value ?? "";
  const kinds = agents.map((a) => a.id).filter((id) => id !== "shell");
  const first = kinds.includes("claude") ? "claude" : (kinds[0] ?? "claude");
  const second = kinds.find((id) => id !== first) ?? first;
  return [first, second].map((agent) => ({ machine, workspace, agent, choice: "", custom: "" }));
}

// modelOf is the model a lane asks for, "" for the agent's own default
export const modelOf = (lane: LaneDraft): string => (lane.choice === CUSTOM ? lane.custom.trim() : lane.choice);

// requestOf is the lanes as the server takes them
export const requestOf = (lanes: LaneDraft[]): LaneRequest[] => lanes.map((l) => ({ machine: l.machine, workspace: l.workspace, agent: l.agent, model: modelOf(l) }));

// whyNot says why the arena cannot start yet, "" when it can
export function whyNot(prompt: string, lanes: LaneDraft[]): string {
  if (!prompt.trim()) return t("Write the prompt every agent gets");
  if (lanes.length < MIN_LANES) return t("An arena needs at least two agents");
  if (lanes.length > MAX_LANES) return t("An arena has at most six agents");
  const missing = lanes.findIndex((l) => !l.machine || !l.workspace || !l.agent);
  if (missing >= 0) return t("Lane {n} needs a machine and a repository", { n: missing + 1 });
  if (lanes.some((l) => l.choice === CUSTOM && !l.custom.trim())) return t("Type the model's id, or pick the default");
  return "";
}

// statWords is a lane's work at a glance: "+12 −3 · 4 files"
export function statWords(stat: Stat | undefined): string {
  if (!stat) return "";
  if (!stat.files) return t("no changes yet");
  return `+${stat.added} −${stat.removed} · ${tn(stat.files, "{n} file", "{n} files")}`;
}

// paneOf is a lane's pane in the roster, while herdr still has it
export function paneOf(machines: Machine[], lane: Lane): Pane | undefined {
  if (!lane.pane) return undefined;
  for (const s of machines.find((m) => m.id === lane.machine)?.sessions ?? []) for (const tab of s.tabs) for (const p of tab.panes) if (p.id === lane.pane) return p;
  return undefined;
}

// runSeconds is how long a lane has run: to now while its agent works or starts, to its last turn once it rests;
// nothing for a lane whose pane is gone, closed or never there
export function runSeconds(lane: Lane, pane: Pane | undefined, now: number): number {
  if (!lane.started || (!pane && lane.state !== "starting")) return 0;
  const from = Date.parse(lane.started);
  if (Number.isNaN(from)) return 0;
  const state = pane?.radar?.state;
  const to = state === "working" || state === "blocked" || lane.state === "starting" ? now : (pane?.radar?.last_active ?? now);
  return Math.max(0, Math.round((to - from) / 1000));
}

// laneTone is how a card wears its lane's state: the kept one green, a closed or failed one dim or red
export function laneTone(lane: Lane): "kept" | "closed" | "failed" | "" {
  if (lane.state === "kept") return "kept";
  if (lane.state === "discarded") return "closed";
  if (lane.state === "failed") return "failed";
  return "";
}

// keepWords is what the confirm says before a lane is kept: which lanes close and that their work is saved first
export function keepWords(lanes: Lane[], keep: number): string {
  const others = lanes.filter((l) => l.id !== keep && l.state !== "discarded" && (l.dir || l.workspace_id));
  if (others.length === 0) return t("Keep this lane? No other lane is open.");
  return tn(
    others.length,
    "Keep this lane and close the other one? Its work — commits and uncommitted changes — is saved as a patch in remuda's state folder first; then its agent stops, and its worktree and branch are removed.",
    "Keep this lane and close the other {n}? Their work — commits and uncommitted changes — is saved as patches in remuda's state folder first; then their agents stop, and their worktrees and branches are removed.",
  );
}
