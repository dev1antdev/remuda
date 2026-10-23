// The next agent that needs the person — a question open, a watchdog note, a finish not looked at — on Alt+N and from the
// palette: on the Wall the tile takes the wall, elsewhere the pane opens on its chat; the roster's order, round and round.

import { useCallback, useEffect } from "react";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { nextNeedingMe, tileKey } from "@/features/roster/wall";
import { usePref } from "@/lib/prefs";
import { useCurrent } from "./current";
import { actions, app } from "./store";

export const NEXT_KEY = "Alt+N";

export function useNextNeedingMe(): () => void {
  const { machines } = useRoster();
  const mode = app.use((s) => s.mode);
  const current = useCurrent();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [focus, setFocus] = usePref<string>("wall.focus", "");
  return useCallback(() => {
    const onWall = mode === "wall";
    const from = onWall && focus ? focus : current ? tileKey(current.machine.id, current.pane.id) : "";
    const next = nextNeedingMe(machines, radar, Date.now(), from);
    if (!next) {
      actions.notify("Nobody needs you right now", { brief: true });
      return;
    }
    if (next.key === from) {
      actions.notify("No one else needs you right now", { brief: true });
      return;
    }
    if (onWall) {
      setFocus(next.key);
      return;
    }
    actions.selectPane(next.machine.id, next.pane.id);
    actions.setLens("chat");
  }, [machines, mode, current, radar, focus, setFocus]);
}

// useNextKey binds Alt+N by key code, whatever the layout; a terminal keeps its own keys and a dialog keeps the focus
export function useNextKey(go: () => void): void {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== "KeyN" || !e.altKey || e.metaKey || e.ctrlKey || e.shiftKey) return;
      if ((e.target as Element | null)?.closest?.(".xterm") || app.get().dialog) return;
      e.preventDefault();
      go();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [go]);
}
