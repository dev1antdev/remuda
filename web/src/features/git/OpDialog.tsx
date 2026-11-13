// The dialog one of git's writes asks before it runs, after git-graph's: its words, the names to type, the box to tick,
// the mode to pick, red where it loses or rewrites something; git's own words stay in it when git refuses, so the
// person can put it right. A branch git will not delete because it is not merged asks once more, in red.

import { useState, type FormEvent } from "react";
import { errMsg } from "@/lib/text";
import { Dialog, Icon } from "@/ui";
import { runOp, type Change, type OpResult } from "./api";
import type { Context } from "./menus";
import { dangerous, initialValues, loseWords, problemOf, requestOf, specFor, type Values, type Writing } from "./ops";
import { t } from "@/lib/i18n";

export function OpDialog({ machineId, dir, action, ctx, changes, onDone, onClose }: {
  machineId: string;
  dir: string;
  action: Writing;
  ctx: Context;
  changes: Change[] | undefined;
  onDone: (result: OpResult, values: Values) => void; // it went, or stopped on conflicts: the dialog closes
  onClose: () => void;
}) {
  const [forced, setForced] = useState(false);
  const spec = specFor(action, ctx, changes, forced);
  const [values, setValues] = useState<Values>(() => initialValues(spec));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const red = dangerous(spec, values);
  const losing = spec.lose && values.mode === "hard";
  const set = (id: string, value: string | boolean) => {
    setValues((v) => ({ ...v, [id]: value }));
    setError("");
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    const problem = problemOf(spec, values);
    if (problem) return setError(problem);
    setBusy(true);
    setError("");
    try {
      const result = await runOp(machineId, dir, requestOf(action, values, ctx, forced));
      if (result.ok || result.operation) return onDone(result, values);
      if (result.not_merged && action.type === "branch-delete" && !forced) {
        setForced(true);
        return;
      }
      setError(result.output || t("git refused."));
    } catch (failure) {
      setError(errMsg(failure));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog title={spec.title} as="form" onSubmit={submit} onClose={onClose} className="git-dialog">
      {spec.detail && <p className="git-dialog-detail">{spec.detail}</p>}
      {spec.inputs?.map((input, i) => (
        <label key={input.id}>
          {input.label}
          <input autoFocus={i === 0} value={String(values[input.id] ?? "")} onChange={(e) => set(input.id, e.target.value)} spellCheck={false} autoCapitalize="off" autoCorrect="off" />
        </label>
      ))}
      {spec.choice && (
        <fieldset className="git-choice">
          <legend>{spec.choice.label}</legend>
          {spec.choice.options.map((option) => (
            <label key={option.value} className={`git-choice-row${values[spec.choice!.id] === option.value ? " on" : ""}${option.danger ? " is-danger" : ""}`}>
              <input type="radio" name={spec.choice!.id} checked={values[spec.choice!.id] === option.value} onChange={() => set(spec.choice!.id, option.value)} />
              <b>{option.label}</b>
              <span>{option.detail}</span>
            </label>
          ))}
        </fieldset>
      )}
      {losing && <p className="git-lose">{loseWords(spec.lose!)}</p>}
      {spec.checks?.map((check) => (
        <label key={check.id} className="check-row">
          <input type="checkbox" checked={!!values[check.id]} onChange={(e) => set(check.id, e.target.checked)} />
          {check.label}
        </label>
      ))}
      {error && (
        <pre className="git-dialog-error" role="alert">
          {error}
        </pre>
      )}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className={`btn ${red ? "danger" : "primary"}`} disabled={busy}>
          {busy ? (
            <>
              <Icon name="refresh-cw" size={12} className="spin" /> {t("Working…")}
            </>
          ) : (
            spec.confirm
          )}
        </button>
      </div>
    </Dialog>
  );
}
