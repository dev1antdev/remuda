// The selection of several tasks at once: a set of ids and the anchor the next ⇧-click ranges from.
// ⌘-click toggles one, ⇧-click takes the rows between the anchor and the row, a tap does the same once a
// long press on a phone started the selection; the list that holds it drops the ids that left the screen.

export type Selection = { ids: number[]; anchor: number | null };

export const NONE: Selection = { ids: [], anchor: null };

export type Pick = "open" | "toggle" | "range";

// pickHow reads a click: ⇧ ranges, ⌘ or Ctrl toggles, a plain click toggles too while a phone's selection is on, else opens
export function pickHow(e: { metaKey: boolean; ctrlKey: boolean; shiftKey: boolean }, selecting: boolean): Pick {
  if (e.shiftKey) return "range";
  if (e.metaKey || e.ctrlKey || selecting) return "toggle";
  return "open";
}

export function isSelected(sel: Selection, id: number): boolean {
  return sel.ids.includes(id);
}

// toggle takes a task in or out; either way it is the anchor of the next range
export function toggle(sel: Selection, id: number): Selection {
  const ids = sel.ids.includes(id) ? sel.ids.filter((x) => x !== id) : [...sel.ids, id];
  return { ids, anchor: id };
}

// range adds every task listed between the anchor and the one clicked, in the order listed; without an anchor
// on the list it is a toggle, and the anchor stays where it was
export function range(sel: Selection, listed: number[], id: number): Selection {
  const from = sel.anchor === null ? -1 : listed.indexOf(sel.anchor);
  const to = listed.indexOf(id);
  if (from < 0 || to < 0) return toggle(sel, id);
  const span = listed.slice(Math.min(from, to), Math.max(from, to) + 1);
  const ids = [...sel.ids, ...span.filter((x) => !sel.ids.includes(x))];
  return { ids, anchor: sel.anchor };
}

// prune keeps only the tasks still listed; the same selection comes back when nothing left
export function prune(sel: Selection, listed: number[]): Selection {
  const ids = sel.ids.filter((x) => listed.includes(x));
  const anchor = sel.anchor !== null && listed.includes(sel.anchor) ? sel.anchor : null;
  return ids.length === sel.ids.length && anchor === sel.anchor ? sel : { ids, anchor };
}

// selectionWords says how many: "3 selected"
export function selectionWords(n: number): string {
  return `${n} selected`;
}
