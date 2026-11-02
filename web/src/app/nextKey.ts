// Whether a key press asks for the next agent that needs the person: Alt+N by key code, whatever the layout, never
// over the terminal, which keeps its own keys, and on a Mac never over a field either: there Option+N is the dead key
// of the tilde (ñ, ã), so a person typing Spanish in the composer must keep it. The pure part of app/next.ts.

export type KeyPress = {
  code: string;
  altKey: boolean;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  target: { closest?: (selector: string) => unknown } | null;
};

// FIELDS are the places Option and a letter type a character on a Mac
export const FIELDS = "input, textarea, select, [contenteditable]";

export function nextKeyAsked(press: KeyPress, dialogOpen: boolean, mac: boolean): boolean {
  if (press.code !== "KeyN" || !press.altKey || press.metaKey || press.ctrlKey || press.shiftKey) return false;
  if (dialogOpen || press.target?.closest?.(".xterm")) return false;
  return !(mac && press.target?.closest?.(FIELDS));
}
