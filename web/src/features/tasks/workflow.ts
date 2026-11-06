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

// Marks is what a column's flags say: where tasks go by themselves, and the Queued column only the person fills
export type Marks = { key: number; kind: string; on_new: boolean; on_start: boolean; on_finish: boolean; on_agent: boolean; on_queue?: boolean };
export type Mark = "on_agent" | "on_new" | "on_queue" | "on_start" | "on_finish";
const BY_THEMSELVES: Mark[] = ["on_agent", "on_new", "on_start", "on_finish"];

// withMark sets one mark on a column and takes it off the others; the Queued mark never shares a column with a mark
// that puts tasks there by themselves, nor sits on a done column, so turning one on takes the other off
export function withMark<R extends Marks>(rows: R[], key: number, mark: Mark, on: boolean): R[] {
  return rows.map((r) => {
    const next: R = { ...r, [mark]: r.key === key ? on : on ? false : r[mark] };
    if (r.key !== key || !on) return next;
    if (mark === "on_queue") {
      for (const other of BY_THEMSELVES) next[other] = false;
      if (r.kind === "done") next.kind = "open";
    } else next.on_queue = false;
    return next;
  });
}

// withKind changes a column's kind; a done column is never the Queued one
export function withKind<R extends Marks>(rows: R[], key: number, kind: string): R[] {
  return rows.map((r) => (r.key !== key ? r : { ...r, kind, on_queue: kind === "done" ? false : r.on_queue }));
}
