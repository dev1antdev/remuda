// A project's columns as the settings edit them: a removed column keeps its row so the choice of where its tasks go
// sits beside it, and that choice holds only while the column it names is kept.

export type EditedColumn = { id: number; removed?: boolean; to?: number }; // id 0: a column added in this edit

// moveTarget is where a removed column's tasks go: the column chosen while it is kept, else the first saved column kept
export function moveTarget(row: EditedColumn, rows: EditedColumn[]): number {
  const survivors = rows.filter((r) => !r.removed && r.id !== 0);
  return row.to !== undefined && survivors.some((s) => s.id === row.to) ? row.to : (survivors[0]?.id ?? 0);
}

// movesOf is what the save sends with the columns: every removed column and where its tasks go
export function movesOf(rows: EditedColumn[]): Record<number, number> {
  const moves: Record<number, number> = {};
  for (const r of rows) if (r.removed) moves[r.id] = moveTarget(r, rows);
  return moves;
}
