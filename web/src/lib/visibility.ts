// The page counts as visible while this document is, or any other document drawn from it: the pop-out window
// stays on screen when the tab behind it is hidden, and the polls that feed it must go on. Inside the Mac app it is
// always visible: the menu bar and the Dock badge are drawn from its roster while the window is hidden.

import { inShell } from "./shell";

const others = new Set<Document>();
const watchers = new Set<() => void>();

const notify = () => {
  for (const fn of watchers) fn();
};

if (typeof document !== "undefined") document.addEventListener("visibilitychange", notify);

// alsoVisible adds a document of ours to the test; the return takes it out again.
export function alsoVisible(doc: Document): () => void {
  others.add(doc);
  doc.addEventListener("visibilitychange", notify);
  return () => {
    others.delete(doc);
    doc.removeEventListener("visibilitychange", notify);
    notify();
  };
}

export const pageVisible = () =>
  inShell() || document.visibilityState === "visible" || [...others].some((doc) => doc.visibilityState === "visible");

// watchVisibility calls back whenever any document of ours changes visibility or leaves; the return stops it.
export function watchVisibility(fn: () => void): () => void {
  watchers.add(fn);
  return () => {
    watchers.delete(fn);
  };
}
