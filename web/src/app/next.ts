// The next agent that needs the person — a question open, a watchdog note, a finish not looked at — on Alt+N and from the
// palette: its pane opens as a notification opens it; the roster's order, round and round.
// And Alt+] / Alt+[, every agent one after another in the sidebar's order.

import { useCallback, useEffect, useRef } from "react";
import { PINS_KEY, agentKey, stepAgent, walkOrder } from "@/features/roster/sidebarAgents";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { needingMe } from "./badge";
import { isMac, usePref } from "@/lib/prefs";
import { useCurrent } from "./current";
import { nextKeyAsked, walkKeyAsked, type KeyPress } from "./nextKey";
import { actions, app } from "./store";
import { t } from "@/lib/i18n";

export const NEXT_KEY = "Alt+N";

export function useNextNeedingMe(): () => void {
  const { machines } = useRoster();
  const current = useCurrent();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  return useCallback(() => {
    const from = current ? agentKey(current.machine.id, current.pane.id) : "";
    const next = stepAgent(needingMe(machines, radar, Date.now()), from, 1);
    if (!next) {
      actions.notify(t("Nobody needs you right now"), { brief: true });
      return;
    }
    if (agentKey(next.machineId, next.paneId) === from) {
      actions.notify(t("No one else needs you right now"), { brief: true });
      return;
    }
    actions.openPane(next.machineId, next.paneId);
  }, [machines, current, radar]);
}

// useWalkKeys binds Alt+] and Alt+[ by key code: the next or the previous agent in the order the sidebar draws them,
// the pinned ones first, round and round (walkOrder in features/roster/agentLines.ts); the rules are Alt+N's
export function useWalkKeys(): void {
  const { machines } = useRoster();
  const current = useCurrent();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [pins] = usePref<string[]>(PINS_KEY, []);
  const share = useSharing();
  const from = current ? agentKey(current.machine.id, current.pane.id) : "";
  const walk = useRef({ machines, radar, pins, from, share });
  walk.current = { machines, radar, pins, from, share };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const press: KeyPress = { code: e.code, altKey: e.altKey, metaKey: e.metaKey, ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, target: e.target as Element | null };
      const step = walkKeyAsked(press, !!app.get().dialog, isMac);
      if (!step) return;
      e.preventDefault();
      const { machines, radar, pins, from, share } = walk.current;
      const next = stepAgent(walkOrder(machines, radar, pins, share.sessionHidden), from, step);
      if (next) actions.selectPane(next.machineId, next.paneId);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);
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
