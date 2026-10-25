// The form that makes or edits a budget: its scope (everything, a machine, a folder on it), the sum, the period,
// a note. The list on the server is replaced whole, so the roster's copy is what the save is built from.

import { useMemo, useState } from "react";
import type { Machine } from "@/features/roster/api";
import { folderOptions, machineOptions } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { FormDialog, Icon, Select, useSubmit } from "@/ui";
import { putBudgets, type BudgetStatus, type Period } from "./api";
import { PERIODS } from "./budgets";

const EVERY = "*";

export default function BudgetDialog({ machines, budget, onClose, onSaved, ask }: {
  machines: Machine[];
  budget: BudgetStatus | null;
  onClose: () => void;
  onSaved: () => void;
  ask: (text: string, label: string, run: () => void) => void;
}) {
  const { budgets } = useRoster();
  const [machine, setMachine] = useState(budget?.machine || EVERY);
  const [dir, setDir] = useState(budget?.dir || EVERY);
  const [limit, setLimit] = useState(budget ? String(budget.limit) : "");
  const [period, setPeriod] = useState<Period>(budget?.period ?? "day");
  const [note, setNote] = useState(budget?.note ?? "");
  const folders = useMemo(() => folderOptions(machines, machine === EVERY ? "all" : machine), [machines, machine]);
  const others = (budgets ?? []).filter((b) => b.id !== budget?.id);
  const sum = Number(limit.replace(",", "."));
  const valid = Number.isFinite(sum) && sum > 0;
  const { busy, err, run } = useSubmit(async () => {
    await putBudgets([
      ...others,
      { id: budget?.id ?? "", machine: machine === EVERY ? "" : machine, dir: machine === EVERY || dir === EVERY ? "" : dir, limit: sum, period, note: note.trim() },
    ]);
    onSaved();
  });
  const remove = () =>
    ask("Remove this budget? Nothing else changes: the spend stays what it is.", "Remove", () => {
      void putBudgets(others).then(onSaved);
    });
  const folderIcon = <Icon name="folder" size={14} />;

  return (
    <FormDialog
      title={budget ? "Edit the budget" : "Add a budget"}
      icon="chart"
      hint="A limit on what the agents cost in a period, at Claude Code's own API prices — on a subscription a measure, not a bill. The usage pill turns amber at 80 %, and a push comes at 80 % and at the limit."
      onClose={onClose}
      submit={{ busy, err, run }}
      submitLabel={budget ? "Save" : "Add"}
    >
      <div className="budget-fields">
        <label>
          Machine
          <Select
            value={machine}
            onChange={(m) => {
              setMachine(m);
              setDir(EVERY);
            }}
            options={[{ value: EVERY, label: "Everything", icon: <Icon name="layers" size={14} /> }, ...machineOptions(machines, { onlyConnected: false })]}
          />
        </label>
        {machine !== EVERY && (
          <label className="grow">
            Folder
            <Select
              value={dir}
              onChange={setDir}
              options={[{ value: EVERY, label: "Every folder", icon: folderIcon }, ...(folders.some((f) => f.value === dir) || dir === EVERY ? folders : [{ value: dir, label: dir, icon: folderIcon }, ...folders])]}
            />
          </label>
        )}
      </div>
      <div className="budget-fields">
        <label>
          Limit, USD
          <input type="number" inputMode="decimal" min={0.01} step="any" value={limit} onChange={(e) => setLimit(e.target.value)} placeholder="20" required autoFocus={!budget} />
        </label>
        <label className="grow">
          Period
          <Select value={period} onChange={setPeriod} options={PERIODS} />
        </label>
      </div>
      <label>
        <span>
          Note <span className="dim">(optional)</span>
        </span>
        <input value={note} onChange={(e) => setNote(e.target.value)} placeholder="What this budget is for" />
      </label>
      {budget && (
        <div className="budget-remove">
          <button type="button" className="btn danger sm" onClick={remove} disabled={busy}>
            Remove this budget
          </button>
        </div>
      )}
      {!valid && limit !== "" && <p className="dim budget-hint">The limit must be above zero.</p>}
    </FormDialog>
  );
}
