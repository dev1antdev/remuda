// Full screen for a lens: the header, the sidebar and the files rail go, the tab strip stays with its tools, and the
// browser's own full screen is asked for where it exists. The toggle sits in the tab strip's tools slot whatever the
// lens, so nothing in the strip moves; Esc outside a field or the terminal leaves, and so does the browser's own exit.

import { useEffect } from "react";
import { Icon } from "@/ui";
import { escapeIsFree, overlayOpen } from "./escape";
import { actions, app } from "./store";

export function enterFull(): void {
  actions.setFull(true);
  const root = document.documentElement;
  if (!document.fullscreenElement && root.requestFullscreen) root.requestFullscreen().catch(() => {});
}

export function leaveFull(): void {
  actions.setFull(false);
  if (document.fullscreenElement && document.exitFullscreen) document.exitFullscreen().catch(() => {});
}


// useFullKeys leaves full screen on an Esc nothing else took — not a field's, the terminal's (where Esc belongs to the
// app), a menu's, a dialog's, the confirm's or the file viewer's (escape.ts) — and when the browser's own full screen ends
export function useFullKeys(): void {
  const full = app.use((s) => s.full);
  useEffect(() => {
    if (!full) return;
    const onKey = (e: KeyboardEvent) => {
      if (!escapeIsFree({ key: e.key, defaultPrevented: e.defaultPrevented, target: e.target as Element | null }, overlayOpen(app.get()))) return;
      leaveFull();
    };
    const onChange = () => {
      if (!document.fullscreenElement) actions.setFull(false);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("fullscreenchange", onChange);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("fullscreenchange", onChange);
    };
  }, [full]);
}
