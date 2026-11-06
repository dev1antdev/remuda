// The pane on screen: the one requested when the roster still has it, else the first agent pane, else the first pane
// (shownPane.ts: the floating terminal's own shell is never a fallback).

import { useMemo } from "react";
import type { Machine } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { shownPane, type Located } from "./shownPane";
import { app } from "./store";

export type { Located } from "./shownPane";

export function allPanes(machines: Machine[]): Located[] {
  const out: Located[] = [];
  for (const machine of machines) for (const session of machine.sessions) for (const tab of session.tabs) for (const pane of tab.panes) out.push({ machine, session, tab, pane });
  return out;
}

export function useAllPanes(): Located[] {
  const { machines } = useRoster();
  return useMemo(() => allPanes(machines), [machines]);
}

export function useCurrent(): Located | null {
  const panes = useAllPanes();
  const requested = app.use((s) => s.requested);
  return useMemo(() => shownPane(panes, requested), [panes, requested]);
}

// the folder a pane works in: the workspace's checkout, else the pane's cwd
export const folderOf = (x: Located) => x.session.dir || x.pane.cwd;
