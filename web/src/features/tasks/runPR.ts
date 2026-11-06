// The pull request a task's run noted, and how it stands while the run's pane is still on the roster naming that link.

import type { Machine, Pane, PRStatus } from "@/features/roster/api";
import type { Run } from "./api";

export type RunPR = { url: string; label: string; status: PRStatus | null };

// paneOf finds a pane on the roster by its machine and id, null once it is gone
export function paneOf(machines: Machine[], machineId: string, paneId: string): Pane | null {
  const machine = machines.find((m) => m.id === machineId);
  if (!machine) return null;
  for (const session of machine.sessions) {
    for (const tab of session.tabs ?? []) {
      const pane = (tab.panes ?? []).find((p) => p.id === paneId);
      if (pane) return pane;
    }
  }
  return null;
}

// runPR is the request the run noted; its state rides along only while the pane still names that very link
export function runPR(run: Run | null | undefined, pane: Pane | null): RunPR | null {
  if (!run?.pr_url) return null;
  const status = pane?.pr && pane.pr.url === run.pr_url ? (pane.pr.status ?? null) : null;
  return { url: run.pr_url, label: run.pr_label || "PR", status };
}
