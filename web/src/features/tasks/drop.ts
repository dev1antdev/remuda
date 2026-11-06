// Where a card dragged over a column lands. The pointer's place is an index among the column's other cards — the one
// being dragged not counted, since it is about to leave its row — and both the drop and the marker read it: the card to
// sit behind, and the row of the column as listed that the marker is drawn above.

// dropAfter is the card a drop at `index` puts the dragged one behind, 0 for the top; null when that is where it already sits
export function dropAfter(column: number[], dragged: number, index: number): { after: number } | null {
  const was = column.indexOf(dragged);
  if (was >= 0 && index === was) return null;
  const others = column.filter((id) => id !== dragged);
  const at = Math.min(index, others.length);
  return { after: at <= 0 ? 0 : others[at - 1]! };
}

// markerSlot is the row of the column as listed that a drop at `index` sits above, the list's length for its end:
// past the dragged card the others' index runs one behind the rows
export function markerSlot(column: number[], dragged: number, index: number): number {
  const was = column.indexOf(dragged);
  return was >= 0 && index > was ? index + 1 : index;
}
