// Which pane is on screen for a request: the one asked for while the roster has it, else the first agent pane, else the
// first pane. The fallbacks leave out the floating terminal's shell, which is the drawer's even once an agent was
// started in it. The pure part of app/current.ts.

import type { Machine, Pane, Session, Tab } from "@/features/roster/api";
import type { Selection } from "./store";

export type Located = { machine: Machine; session: Session; tab: Tab; pane: Pane };

const matches = (x: Located, r: Selection) => x.machine.id === r.machineId && x.pane.id === r.paneId;

export function shownPane(panes: Located[], requested: Selection | null): Located | null {
  return (requested && panes.find((x) => matches(x, requested))) ?? panes.find((x) => x.pane.agent && !x.session.scratch) ?? panes.find((x) => !x.session.scratch) ?? null;
}
