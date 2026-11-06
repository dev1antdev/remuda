// Whether an Esc is the dialog's to close it: not one a tooltip, a menu or another layer inside it already used (Radix
// marks the one it takes with preventDefault), not while a Select's list is open, and only for the topmost dialog. The
// pure part of useEscape in ui/Dialog.tsx; app/escape.ts is the page's own rule.

export type DialogEscapePress = { key: string; defaultPrevented: boolean };

export function dialogTakesEscape(press: DialogEscapePress, selectOpen: boolean, topmost: boolean): boolean {
  return press.key === "Escape" && !press.defaultPrevented && !selectOpen && topmost;
}
