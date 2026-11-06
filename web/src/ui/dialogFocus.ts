// Where a dialog gives the focus back when it closes: the element that had it as the dialog opened — or, for a row of a
// menu, which closes and leaves the page as the dialog opens, the button that opened that menu (Radix names it as the
// menu's aria-labelledby). Focusing the row would focus nothing and leave the focus on the page's body. The pure part
// of ui/Dialog.tsx.

export type FocusHolder = { closest?: (selector: string) => { getAttribute: (name: string) => string | null } | null };

export function focusReturn<T extends FocusHolder>(active: T | null, byId: (id: string) => T | null): T | null {
  const trigger = active?.closest?.('[role="menu"]')?.getAttribute("aria-labelledby");
  return (trigger ? byId(trigger) : null) ?? active;
}

// backTo is where the focus goes as a dialog closes: its opener while that is still on the page, else where the dialog it
// was opened from would have sent it — a dialog the palette opened outlives the palette and its field
export function backTo<T extends { isConnected?: boolean }>(opener: T | null, outer: T | null): T | null {
  return opener?.isConnected ? opener : outer;
}
