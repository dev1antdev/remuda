// The page counts as visible while its document is. Inside the Mac app it is always visible: the menu bar and the Dock
// badge are drawn from its roster while the window is hidden.

import { inShell } from "./shell";

const watchers = new Set<() => void>();

const notify = () => {
  for (const fn of watchers) fn();
};

if (typeof document !== "undefined") document.addEventListener("visibilitychange", notify);

export const pageVisible = () => inShell() || document.visibilityState === "visible";

// documentVisible says whether this very document is on screen, whatever the Mac app's menu bar needs: what only this
// document draws — the sidebar's "Needs you" — reads nothing while it is hidden
export const documentVisible = () => document.visibilityState === "visible";

// watchVisibility calls back whenever the document changes visibility; the return stops it.
export function watchVisibility(fn: () => void): () => void {
  watchers.add(fn);
  return () => {
    watchers.delete(fn);
  };
}
