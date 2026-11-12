// The concierge's place, the pure part: its workspace taken out of the roster so no other screen sees it, the pane its
// conversation is in, what to say while it comes up, and the questions offered when the conversation is empty.

import type { ChatResponse, Turn } from "@/features/chat/api";
import type { Machine, Pane, Session } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import type { ConciergeMachine, ConciergeView } from "./api";

// ConciergeEntry is a concierge's workspace on a machine, as the roster carries it
export type ConciergeEntry = { machineId: string; session: Session };

// splitConcierge takes every concierge's workspace out of the machines: the sidebar, the wall and its zoo, the palette,
// the pickers, the badge and the Mac app's menu read the fleet without it, and its place reads the entries alone
export function splitConcierge(machines: Machine[]): { machines: Machine[]; concierges: ConciergeEntry[] } {
  const concierges: ConciergeEntry[] = [];
  let changed = false;
  const rest = machines.map((machine) => {
    const own = machine.sessions.filter((s) => s.concierge);
    if (own.length === 0) return machine;
    changed = true;
    for (const session of own) concierges.push({ machineId: machine.id, session });
    return { ...machine, sessions: machine.sessions.filter((s) => !s.concierge) };
  });
  return { machines: changed ? rest : machines, concierges };
}

// conciergePane is the pane the concierge talks in on a machine: the one a start named, else its workspace's active
// tab's focused pane, else its first
export function conciergePane(concierges: ConciergeEntry[], machineId: string, paneId?: string): Pane | undefined {
  const entry = concierges.find((c) => c.machineId === machineId);
  if (!entry) return undefined;
  const panes = entry.session.tabs.flatMap((tab) => tab.panes);
  const active = entry.session.tabs.find((tab) => tab.id === entry.session.active_tab_id)?.panes ?? [];
  return panes.find((p) => p.id === paneId) ?? active.find((p) => p.focused) ?? active[0] ?? panes[0];
}

// needsStart says whether the place starts the concierge when it opens: nothing stands, or its pane is a bare shell
// again (herdr brought it back after a reboot, or the agent left); a pane the roster does not list yet is waited for
export function needsStart(running: boolean, pane: Pane | undefined): boolean {
  return !running || (pane !== undefined && !pane.agent);
}

// shownTurns is the conversation to draw: none while the chat still reads the one a New conversation set aside (the
// folder's newest file, until the fresh one is written)
export function shownTurns(chat: ChatResponse | null | undefined, setAside: string | null): Turn[] {
  if (!chat || (setAside && chat.session_id === setAside)) return [];
  return chat.turns;
}

// comingUp is what the place says while the concierge is not ready to talk: starting on the machine, or its agent
// starting in its pane; "" once Claude Code runs there
export function comingUp(machineLabel: string, pane: Pane | undefined): string {
  if (!pane) return t("Starting the concierge on {machine}…", { machine: machineLabel });
  if (!pane.agent) return t("Starting Claude Code on {machine}…", { machine: machineLabel });
  return "";
}

// machineChoices is the picker: the machines the concierge can run on, an away one saying so
export function machineChoices(view: ConciergeView): { value: string; label: string; hint?: string }[] {
  return view.machines.map((m: ConciergeMachine) => ({ value: m.id, label: m.label, hint: m.connected ? undefined : t("away") }));
}

// labelOf is a machine's name as the picker shows it, its id when the picker does not list it
export function labelOf(view: ConciergeView | null | undefined, machineId: string): string {
  return view?.machines.find((m) => m.id === machineId)?.label ?? machineId;
}

// HINTS are the questions offered in an empty conversation: a click puts one in the box, never sends it
export const HINTS = [t("What did the agents finish today?"), t("Who is working on auth?"), t("File a task for the flaky web tests")];
