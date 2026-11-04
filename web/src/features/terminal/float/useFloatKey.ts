// Ctrl+` opens the floating terminal on the machine of the pane on screen, or hides it; heard before anything else on
// the page, so a terminal with the focus — the drawer's own included — passes it on instead of typing it.

import { useEffect, useRef } from "react";
import { actions, app } from "@/app/store";
import type { Machine } from "@/features/roster/api";
import { floatMachine, isFloatKey } from "./float";
import { t } from "@/lib/i18n";

// toggleFloat hides the drawer when it is open, else opens it on the machine the rules pick
export function toggleFloat(machines: Machine[], current: string | null) {
  if (app.get().float) {
    actions.closeFloat();
    return;
  }
  const machineId = floatMachine(null, current, machines);
  if (machineId) actions.openFloat(machineId);
  else actions.notify(t("No machine is connected for a terminal"), { error: true });
}

export function useFloatKey(machines: Machine[], current: string | null, enabled: boolean) {
  const live = useRef({ machines, current });
  live.current = { machines, current };
  useEffect(() => {
    if (!enabled) return;
    const onKey = (e: KeyboardEvent) => {
      if (!isFloatKey(e)) return;
      e.preventDefault();
      e.stopPropagation();
      toggleFloat(live.current.machines, live.current.current);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [enabled]);
}
