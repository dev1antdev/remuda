// The address says where the page is: ?machine=&pane=&mode=&lens=. A reload or a shared link lands there;
// a tapped notification carries machine and pane the same way, or ?panel= for a panel of the page (a digest
// opens the standup), read once at load and never written back. A link to a turn adds &session=&turn=, read once
// at load into a jump and dropped by the first write, so a reload lands at the bottom as usual.

import { readTurnLink, type TurnLinkTarget } from "@/features/chat/turnLink";
import type { Lens, Mode, Panel, Selection } from "./store";

const LENSES: Lens[] = ["chat", "terminal", "changes"];
const MODES: Mode[] = ["herdr", "wall", "tasks", "code", "review"];
const PANELS: Panel[] = ["search", "alerts", "activity", "schedules", "usage", "standup", "concierge"];

// panelOf keeps a value only when it names a panel a link may open
export function panelOf(value: unknown): Panel {
  return PANELS.includes(value as Panel) ? (value as Panel) : null;
}

export function readURL(): { requested: Selection | null; mode?: Mode; lens?: Lens; turn: TurnLinkTarget | null; panel: Panel } {
  const q = new URLSearchParams(location.search);
  const paneId = q.get("pane");
  const mode = q.get("mode") as Mode | null;
  const lens = q.get("lens") as Lens | null;
  return {
    requested: paneId ? { machineId: q.get("machine") || "local", paneId } : null,
    mode: mode && MODES.includes(mode) ? mode : undefined,
    lens: lens && LENSES.includes(lens) ? lens : undefined,
    turn: readTurnLink(location.search),
    panel: panelOf(q.get("panel")),
  };
}

export type Where = { selection: Selection | null; mode: Mode; lens: Lens };

// addressFor is the address a place of the page has: the pane and, in Agents, its lens, the mode when it is not Agents;
// "/" for the first mode with nothing selected. The query is read as text, never through URLSearchParams.size, which
// Safari 16 lacks: there it was undefined, so every address came out as "/" and a reload lost the pane.
export function addressFor(at: Where): string {
  const q = new URLSearchParams();
  if (at.selection) {
    q.set("machine", at.selection.machineId);
    q.set("pane", at.selection.paneId);
  }
  if (at.mode !== "herdr") q.set("mode", at.mode);
  if (at.selection && at.mode === "herdr") q.set("lens", at.lens);
  const query = q.toString();
  return query ? `/?${query}` : "/";
}

// writeURL replaces the address in place, keeping any history state another part of the page put there.
export function writeURL(at: Where) {
  const next = addressFor(at);
  if (location.pathname + location.search !== next) history.replaceState(history.state, "", next);
}
