// The pane on screen: the one requested when the roster still has it, else the first agent pane, else the first pane.

import { useMemo } from "react";
import type { Machine, Pane, Session, Tab } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { app, type Selection } from "./store";

export type Located = { machine: Machine; session: Session; tab: Tab; pane: Pane };

export function allPanes(machines: Machine[]): Located[] {
  const out: Located[] = [];
  for (const machine of machines) for (const session of machine.sessions) for (const tab of session.tabs) for (const pane of tab.panes) out.push({ machine, session, tab, pane });
  return out;
}

export function useAllPanes(): Located[] {
  const { machines } = useRoster();
  return useMemo(() => allPanes(machines), [machines]);
}

const matches = (x: Located, r: Selection) => x.machine.id === r.machineId && x.pane.id === r.paneId;

export function useCurrent(): Located | null {
  const panes = useAllPanes();
  const requested = app.use((s) => s.requested);
  return useMemo(() => (requested && panes.find((x) => matches(x, requested))) ?? panes.find((x) => x.pane.agent) ?? panes[0] ?? null, [panes, requested]);
}

// the folder a pane works in: the workspace's checkout, else the pane's cwd
export const folderOf = (x: Located) => x.session.dir || x.pane.cwd;
