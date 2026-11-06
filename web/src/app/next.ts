// The next agent that needs the person — a question open, a watchdog note, a finish not looked at — on Alt+N and from the
// palette: on the Wall the tile takes the wall, elsewhere the pane opens on its chat; the roster's order, round and round.

import { useCallback, useEffect } from "react";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { nextNeedingMe, tileKey } from "@/features/roster/wall";
import { isMac, usePref } from "@/lib/prefs";
import { useCurrent } from "./current";
import { nextKeyAsked, type KeyPress } from "./nextKey";
import { actions, app } from "./store";
import { t } from "@/lib/i18n";

export const NEXT_KEY = "Alt+N";

export function useNextNeedingMe(): () => void {
  const { machines } = useRoster();
  const mode = app.use((s) => s.mode);
  const current = useCurrent();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [focus, setFocus] = usePref<string>("wall.focus", "");
  const [hidden] = usePref<string[]>("wall.hidden", []);
  return useCallback(() => {
    const onWall = mode === "wall";
    const from = onWall && focus ? focus : current ? tileKey(current.machine.id, current.pane.id) : "";
    const next = nextNeedingMe(machines, radar, Date.now(), from, onWall ? hidden : []);
    if (!next) {
      actions.notify(t("Nobody needs you right now"), { brief: true });
      return;
    }
    if (next.key === from) {
      actions.notify(t("No one else needs you right now"), { brief: true });
      return;
    }
    if (onWall) {
      setFocus(next.key);
      return;
    }
    actions.openPane(next.machine.id, next.pane.id);
  }, [machines, mode, current, radar, focus, setFocus, hidden]);
}

// useNextKey binds Alt+N by key code, whatever the layout; a terminal keeps its own keys, a dialog keeps the focus, and
// a field on a Mac keeps Option+N (nextKey.ts)
export function useNextKey(go: () => void): void {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const press: KeyPress = { code: e.code, altKey: e.altKey, metaKey: e.metaKey, ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, target: e.target as Element | null };
      if (!nextKeyAsked(press, !!app.get().dialog, isMac)) return;
      e.preventDefault();
      go();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [go]);
}
