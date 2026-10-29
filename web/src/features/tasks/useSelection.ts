// The selection's state and the bulk actions on it: which tasks are picked, how a click picks, Esc clears, the rows
// that left the list are dropped, and one action becomes a request per task sent in sequence, with a toast at the end.

import { useCallback, useEffect, useMemo, useState } from "react";
import { actions, app } from "@/app/store";
import { isTouch } from "@/lib/prefs";
import { addTaskLabel, deleteTask, patchTask, type Label, type Status, type Task } from "./api";
import { columnChoices, deleteWords, labelChoices, outcomeWords, plan, runInSequence, type Action, type Request } from "./bulk";
import { NONE, isSelected, prune, range, toggle, type Pick, type Selection } from "./selection";

const send = (r: Request) => (r.op === "patch" ? patchTask(r.id, r.fields) : r.op === "label" ? addTaskLabel(r.id, r.labelId) : deleteTask(r.id));

export function useSelection({ tasks, labels, columns, drop, refresh }: {
  tasks: Task[]; // as listed, the order a range follows
  labels: Label[];
  columns: (t: Task) => Status[]; // a task's project's columns
  drop: (ids: number[]) => void; // the rows leave the list at once (a delete)
  refresh: () => void;
}) {
  const [sel, setSel] = useState<Selection>(NONE);
  const [busy, setBusy] = useState(false);
  const listed = useMemo(() => tasks.map((t) => t.id), [tasks]);
  useEffect(() => setSel((s) => prune(s, listed)), [listed]);
  const count = sel.ids.length;
  const selecting = isTouch && count > 0; // a phone's taps toggle while something is selected

  useEffect(() => {
    if (count === 0) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || (e.target as Element | null)?.closest?.("input, textarea, select, [contenteditable]") || app.get().dialog || app.get().confirm) return;
      setSel(NONE);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [count]);

  const pick = useCallback((id: number, how: Pick) => setSel((s) => (how === "range" ? range(s, listed, id) : toggle(s, id))), [listed]);
  const clear = useCallback(() => setSel(NONE), []);
  // the tasks given ids leave the selection: the ones a bulk start got agents on
  const release = useCallback((ids: number[]) => setSel((s) => prune(s, s.ids.filter((id) => !ids.includes(id)))), []);
  const chosen = useMemo(() => tasks.filter((t) => sel.ids.includes(t.id)), [tasks, sel]);

  const run = async (action: Action) => {
    const { requests, skipped } = plan(action, chosen, columns);
    setBusy(true);
    if (action.kind === "delete") drop(requests.map((r) => r.id));
    const outcome = await runInSequence(requests, send);
    setBusy(false);
    setSel(NONE);
    actions.notify(outcomeWords(action, outcome, skipped), { error: outcome.failed > 0 });
    refresh();
  };
  const act = (action: Action) => {
    if (action.kind === "delete") actions.ask(deleteWords(count), "Delete", () => void run(action));
    else void run(action);
  };

  return {
    count,
    busy,
    selecting,
    picked: (id: number) => isSelected(sel, id),
    pick,
    clear,
    release,
    act,
    chosen,
    columnChoices: columnChoices(chosen, columns),
    labelChoices: labelChoices(chosen, labels),
  };
}
