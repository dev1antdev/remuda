// The address says where the page is: ?machine=&pane=&mode=&lens=. A reload or a shared link lands there;
// a tapped notification carries machine and pane the same way, or ?panel= for a panel of the page (a digest
// opens the standup), read once at load and never written back. A link to a turn adds &session=&turn=, read once
// at load into a jump and dropped by the first write, so a reload lands at the bottom as usual.

import { readTurnLink, type TurnLinkTarget } from "@/features/chat/turnLink";
import type { Lens, Mode, Panel, Selection } from "./store";

const LENSES: Lens[] = ["chat", "terminal", "changes"];
const MODES: Mode[] = ["herdr", "wall", "tasks", "code", "review"];
const PANELS: Panel[] = ["search", "alerts", "activity", "schedules", "usage", "standup"];

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

// writeURL replaces the address in place, keeping any history state another part of the page put there.
export function writeURL(at: { selection: Selection | null; mode: Mode; lens: Lens }) {
  const q = new URLSearchParams();
  if (at.selection) {
    q.set("machine", at.selection.machineId);
    q.set("pane", at.selection.paneId);
  }
  if (at.mode !== "herdr") q.set("mode", at.mode);
  if (at.selection && at.mode === "herdr") q.set("lens", at.lens);
  const next = q.size ? `/?${q}` : "/";
  if (location.pathname + location.search !== next) history.replaceState(history.state, "", next);
}
