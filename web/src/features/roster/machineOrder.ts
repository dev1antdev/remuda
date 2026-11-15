// The machines in the person's order: where a machine dragged by its head lands, a step up or down from its menu, and
// the mark that shows where a drop would put it.

// MACHINE_DRAG is the type a machine's drag carries
export const MACHINE_DRAG = "application/x-remuda-machine";

// Drag is a machine on its way: the one dragged, the one the pointer is over, and whether it would land after it
export type Drag = { id: string; over: string | null; after: boolean };

export const sameOrder = (a: string[], b: string[]): boolean => a.length === b.length && a.every((id, i) => id === b[i]);

// movedBeside is the order with one machine taken out and put beside another: before it, or after it
export function movedBeside(ids: string[], dragged: string, target: string, after: boolean): string[] {
  if (dragged === target || !ids.includes(dragged) || !ids.includes(target)) return ids;
  const rest = ids.filter((id) => id !== dragged);
  const at = rest.indexOf(target) + (after ? 1 : 0);
  return [...rest.slice(0, at), dragged, ...rest.slice(at)];
}

// stepped is the order with one machine a place earlier (-1) or later (1); the order as it is at either end
export function stepped(ids: string[], id: string, by: -1 | 1): string[] {
  const at = ids.indexOf(id);
  const to = at + by;
  if (at < 0 || to < 0 || to >= ids.length) return ids;
  return ids.map((each, i) => (i === at ? (ids[to] ?? each) : i === to ? id : each));
}

// lowerHalf says whether a pointer at y is over the lower half of a box: a drop there lands after it
export const lowerHalf = (y: number, top: number, height: number): boolean => y > top + height / 2;

// inOrder is the machines as an order names them, the ones it does not name after those, as they came
export function inOrder<T extends { id: string }>(machines: T[], order: string[] | null): T[] {
  if (!order) return machines;
  const placed = order.flatMap((id) => machines.filter((machine) => machine.id === id));
  return [...placed, ...machines.filter((machine) => !order.includes(machine.id))];
}

// dropMark is where a machine's block shows the line of a drop: before it, after it, or nowhere — not on the machine
// dragged, and not where the drop would leave the order as it is
export function dropMark(ids: string[], drag: Drag | null, id: string): "before" | "after" | null {
  if (!drag || drag.over !== id || drag.id === id) return null;
  if (sameOrder(movedBeside(ids, drag.id, id, drag.after), ids)) return null;
  return drag.after ? "after" : "before";
}
