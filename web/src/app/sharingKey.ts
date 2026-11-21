// Whether a key press turns the screen-sharing mode on or off: Alt+H by key code, whatever the layout, under the same
// rule as Alt+N — never over the terminal, never with a dialog open, and on a Mac never over a field, where Option+H
// types a character. The pure part of useSharingKey.

import { FIELDS, type KeyPress } from "./nextKey";

export const SHARING_KEY_NAME = "Alt+H";

export function sharingKeyAsked(press: KeyPress, dialogOpen: boolean, mac: boolean): boolean {
  if (press.code !== "KeyH" || !press.altKey || press.metaKey || press.ctrlKey || press.shiftKey) return false;
  if (dialogOpen || press.target?.closest?.(".xterm")) return false;
  return !(mac && press.target?.closest?.(FIELDS));
}
