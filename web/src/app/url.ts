// The address says where the page is: ?machine=&pane=&mode=&lens=. A reload or a shared link lands there;
// a tapped notification carries machine and pane the same way.

import type { Lens, Mode, Selection } from "./store";

const LENSES: Lens[] = ["chat", "terminal", "changes"];
const MODES: Mode[] = ["herdr", "wall", "tasks", "code"];

export function readURL(): { requested: Selection | null; mode?: Mode; lens?: Lens } {
  const q = new URLSearchParams(location.search);
  const paneId = q.get("pane");
  const mode = q.get("mode") as Mode | null;
  const lens = q.get("lens") as Lens | null;
  return {
    requested: paneId ? { machineId: q.get("machine") || "local", paneId } : null,
    mode: mode && MODES.includes(mode) ? mode : undefined,
    lens: lens && LENSES.includes(lens) ? lens : undefined,
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
