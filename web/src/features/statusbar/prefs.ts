// The status bar's prefs, per device and synced with the look: whether it shows, and whether the plan limits say what
// remains rather than what is used. It shows on a desk with a mouse only: a phone's rail lies where it would.

import { readPref, usePref, writePref } from "@/lib/prefs";

export const STATUS_BAR_KEY = "statusbar";

// STATUS_BAR_QUERY is the screen the bar lives on: wider than a phone's, with a mouse
export const STATUS_BAR_QUERY = "(min-width: 921px) and (pointer: fine)";

export type StatusBarPrefs = { on: boolean; remaining: boolean };

export const DEFAULT_STATUS_BAR: StatusBarPrefs = { on: true, remaining: false };

// statusBarOf keeps what a stored value says and fills in the rest
export const statusBarOf = (stored: Partial<StatusBarPrefs> | null | undefined): StatusBarPrefs => ({
  on: typeof stored?.on === "boolean" ? stored.on : DEFAULT_STATUS_BAR.on,
  remaining: typeof stored?.remaining === "boolean" ? stored.remaining : DEFAULT_STATUS_BAR.remaining,
});

export function useStatusBarPrefs(): [StatusBarPrefs, (next: Partial<StatusBarPrefs>) => void] {
  const [stored, setStored] = usePref<StatusBarPrefs>(STATUS_BAR_KEY, DEFAULT_STATUS_BAR);
  return [statusBarOf(stored), (next) => setStored((prev) => ({ ...statusBarOf(prev), ...next }))];
}

// toggleStatusBar shows or hides the bar, from the View menu
export const toggleStatusBar = () => {
  const now = statusBarOf(readPref<StatusBarPrefs>(STATUS_BAR_KEY, DEFAULT_STATUS_BAR));
  writePref(STATUS_BAR_KEY, { ...now, on: !now.on });
};
