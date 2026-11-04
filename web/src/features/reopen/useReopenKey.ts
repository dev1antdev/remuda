// Alt+Z by key code opens the closed agents' list: the browser keeps its own ⌘⇧T, the terminal its keys, a dialog the
// focus, and a field on a Mac its Option+Z (closed.ts).

import { useEffect } from "react";
import { actions, app } from "@/app/store";
import { isMac } from "@/lib/prefs";
import { reopenKeyAsked, type KeyPress } from "./closed";

export function useReopenKey(enabled: boolean): void {
  useEffect(() => {
    if (!enabled) return;
    const onKey = (e: KeyboardEvent) => {
      const press: KeyPress = { code: e.code, altKey: e.altKey, metaKey: e.metaKey, ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, target: e.target as Element | null };
      if (!reopenKeyAsked(press, !!app.get().dialog, isMac)) return;
      e.preventDefault();
      actions.openDialog({ kind: "reopen" });
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [enabled]);
}
