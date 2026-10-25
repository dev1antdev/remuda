// Full screen for a lens: the header, the sidebar and the files rail go, the tab strip stays with its tools, and the
// browser's own full screen is asked for where it exists. The toggle sits in the tab strip's tools slot whatever the
// lens, so nothing in the strip moves; Esc outside a field or the terminal leaves, and so does the browser's own exit.

import { useEffect } from "react";
import { Icon } from "@/ui";
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


// useFullKeys leaves full screen on Esc outside a field or the terminal (where Esc belongs to the app), and when the
// browser's own full screen ends
export function useFullKeys(): void {
  const full = app.use((s) => s.full);
  useEffect(() => {
    if (!full) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || (e.target as Element | null)?.closest?.("input, textarea, select, [contenteditable], .xterm") || app.get().dialog || app.get().panel) return;
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
