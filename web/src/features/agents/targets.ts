// An agent pane from herdr's roster, flattened for the pickers that send text to one.

import type { Machine } from "@/features/roster/api";
import { folderOf } from "@/features/roster/workspaces";
import { VENDOR_NAME } from "@/ui/brand";

export type AgentTarget = {
  machineId: string;
  machineLabel: string;
  sessionName: string;
  paneTitle: string; // the tab's name, or the agent's own title for its conversation
  paneId: string;
  agent: string;
  status: string;
  cwd: string;
  folder?: string; // the workspace's folder, the one the screen-sharing mode hides
};

export const targetKey = (a: { machineId: string; paneId: string }) => `${a.machineId}|${a.paneId}`;

// agentTargets is every agent pane on every machine, in the roster's order.
export function agentTargets(machines: Machine[]): AgentTarget[] {
  const out: AgentTarget[] = [];
  for (const machine of machines)
    for (const session of machine.sessions)
      for (const tab of session.tabs)
        for (const pane of tab.panes) {
          if (!pane.agent) continue;
          out.push({
            machineId: machine.id,
            machineLabel: machine.label,
            sessionName: session.name,
            paneTitle: tab.named ? tab.label : pane.title || tab.label,
            paneId: pane.id,
            agent: pane.agent,
            status: pane.status,
            cwd: pane.cwd,
            folder: folderOf(session),
          });
        }
  return out;
}

// targetWords is how a picker names an agent: its workspace and its tab or conversation, or, while the screen-sharing
// mode hides its folder (masked: the folder's neutral name), that name and the agent's kind, as the tab strip says it
export function targetWords(a: AgentTarget, masked: string | null): string {
  return masked === null ? `${a.sessionName} › ${a.paneTitle}` : `${masked} › ${VENDOR_NAME[a.agent] ?? a.agent}`;
}
