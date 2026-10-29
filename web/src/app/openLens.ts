// Where a pane opens when something other than a click on it brings the person there: a notification's tap, the Mac
// app's menu, an Activity row, Alt+N. The pref `open.lens` names the lens — the terminal unless chosen otherwise — or
// keeps whichever lens the page was on. The sidebar, the tabs and the Wall keep their own ways.

export type OpenLens = "terminal" | "chat" | "keep";
export type PaneLens = "terminal" | "chat" | "changes";

export const OPEN_LENS_KEY = "open.lens";
export const DEFAULT_OPEN_LENS: OpenLens = "terminal";

// openLensOf reads a stored value, the default for anything else
export function openLensOf(value: unknown): OpenLens {
  return value === "chat" || value === "keep" ? value : DEFAULT_OPEN_LENS;
}

// lensToOpen is the lens a pane opens in: the chosen one, or the current one when the choice is to keep it
export function lensToOpen(pref: OpenLens, current: PaneLens): PaneLens {
  return pref === "keep" ? current : pref;
}
