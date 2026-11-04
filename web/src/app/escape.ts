// Whose Escape a press is. A menu takes the Esc that closes it first (Radix marks it with preventDefault), a dialog, the
// confirm, the file viewer or a panel take theirs, a field and the terminal keep their own; only what is left is the
// page's, to leave full screen or give a focused tile back to the grid. The pure part of app/full.tsx and the Wall.

export type EscapePress = { key: string; defaultPrevented: boolean; target: { closest?: (selector: string) => unknown } | null };

export const KEEPS_ESCAPE = "input, textarea, select, [contenteditable], .xterm, [role=menu], .modal-scrim, .fv-scrim, .sel-pop";

// escapeIsFree says whether the page may act on this press: an Escape nothing else took, with no overlay open
export function escapeIsFree(press: EscapePress, overlayOpen: boolean): boolean {
  if (press.key !== "Escape" || press.defaultPrevented || overlayOpen) return false;
  return !press.target?.closest?.(KEEPS_ESCAPE);
}

// OverlayState is what the page's store says is open over it
export type OverlayState = { dialog: unknown; panel: unknown; confirm: unknown; file: unknown };

export const overlayOpen = (s: OverlayState): boolean => !!(s.dialog || s.panel || s.confirm || s.file);
