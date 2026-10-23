// The wall's tiles: which agent panes make one and in what order. Pure functions over the roster, tested beside.

import type { Machine, Pane, Session, Tab } from "./api";
import { displayOf, type RadarPrefs } from "./radar";

export type WallView = "chat" | "screen"; // what a tile shows: the last turns of the conversation, or the terminal as it is
export type WallPrefs = { size: "s" | "m" | "l"; hideIdle: boolean; view: WallView };
export const DEFAULT_WALL: WallPrefs = { size: "m", hideIdle: false, view: "chat" };

// the narrowest a tile gets, per size; the grid fits as many columns as the window allows
export const TILE_MIN: Record<WallPrefs["size"], number> = { s: 360, m: 460, l: 600 };

export type Tile = { key: string; machine: Machine; session: Session; tab: Tab; pane: Pane };
export type Tiles = { shown: Tile[]; hidden: number; idle: number };

export const tileKey = (machineId: string, paneId: string) => `${machineId}|${paneId}`;

// tiles lists every agent pane on a connected machine in a stable order — the machines as the roster has them, the panes as
// herdr has them — so no tile jumps when a status changes; the ones hidden by hand and, when asked, the idle ones past the
// fresh tier are left out and counted
export function tiles(machines: Machine[], hidden: string[], prefs: WallPrefs, radar: RadarPrefs, now: number): Tiles {
  const out: Tiles = { shown: [], hidden: 0, idle: 0 };
  const gone = new Set(hidden);
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const session of machine.sessions)
      for (const tab of session.tabs)
        for (const pane of tab.panes) {
          if (!pane.agent) continue;
          const key = tileKey(machine.id, pane.id);
          if (gone.has(key)) {
            out.hidden++;
            continue;
          }
          if (prefs.hideIdle && isIdle(pane, radar, now)) {
            out.idle++;
            continue;
          }
          out.shown.push({ key, machine, session, tab, pane });
        }
  }
  return out;
}

// isIdle is an agent resting past the fresh tier: one that just stopped still earns its tile
export function isIdle(pane: Pane, radar: RadarPrefs, now: number): boolean {
  const d = displayOf(pane, radar, now);
  return d === "idle" || d === "idle_stale";
}
