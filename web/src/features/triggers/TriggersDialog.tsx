// Triggers: what a pane prints, and what it sets off. The built-in two are said at the top; the person's own rules are
// a list edited in place and saved whole. Opened from a workspace, the dialog lists the rules that look at its folder
// and a new one starts there; from the palette, every rule.

import { useEffect, useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Notice, Tip } from "@/ui";
import { t, tn } from "@/lib/i18n";
import { fetchTriggers, saveTriggers, type Rule } from "./api";
import TriggerForm from "./TriggerForm";
import { draftCheck, looksAt, newRule, ruleWords } from "./triggers";

export default function TriggersDialog({ machineId, dir, workspace, name, onClose }: {
  machineId?: string; // the workspace's machine, when opened from one
  dir?: string; // its folder
  workspace?: string; // its id, for the panes a rule is tried on
  name?: string;
  onClose: () => void;
}) {
  const { machines } = useRoster();
  const { redact } = useSharing(); // under the screen-sharing mode the machines and the folders wear their neutral names
  // the save replaces the list whole, so nothing is saved until the current one has loaded
  const loaded = useQuery(fetchTriggers, []);
  const [list, setList] = useState<Rule[] | null>(null);
  const [open, setOpen] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (loaded.data && list === null) setList(loaded.data.rules);
  }, [loaded.data, list]);

  const machineLabel = (id: string) => redact(machines.find((m) => m.id === id)?.label ?? id);
  const here = (rule: Rule) => !machineId || looksAt(rule, machineId, dir ?? "");
  const rules = list ?? [];
  const shown = rules.map((rule, i) => ({ rule, i })).filter(({ rule, i }) => here(rule) || i === open);
  const others = rules.length - shown.length;
  const max = loaded.data?.max ?? 50;
  const refused = Object.entries(loaded.data?.refused ?? {});
  const editing = open !== null ? rules[open] : undefined;
  const blocked = editing ? draftCheck(editing) : "";

  const change = (i: number, rule: Rule) => setList(rules.map((x, j) => (j === i ? rule : x)));
  const remove = (i: number) => {
    setList(rules.filter((_, j) => j !== i));
    setOpen(null);
  };
  const add = () => {
    setList([...rules, newRule(machineId, dir)]);
    setOpen(rules.length);
  };
  const save = () => {
    if (!list || busy || blocked) return;
    setBusy(true);
    setErr("");
    saveTriggers(list)
      .then(onClose, (e) => setErr(errMsg(e)))
      .finally(() => setBusy(false));
  };

  return (
    <Dialog
      title={name ? t("Triggers in {workspace}", { workspace: redact(name) }) : t("Triggers")}
      icon="zap"
      size="md"
      as="form"
      className="triggers-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        save();
      }}
    >
      <p className="modal-hint">
        {t("herdr watches what each pane prints. Built in, on every pane: a dev server saying where it serves is offered in the corner as a preview once its port listens, and tests that failed send one push per pane — never while its agent is at work or its terminal is open here, then not again for half an hour.")}
      </p>
      <p className="modal-hint">{t("Your own triggers look for a line — text as written, or a regular expression — everywhere or in one folder, and push, note it in the activity, offer a preview or move the task whose agent runs there. Shared by every device that uses this remuda.")}</p>
      {refused.map(([machine, why]) => (
        <Notice key={machine} tone="warn">
          {t("herdr on {machine} refused your triggers, so they are left out there until you change them: {why}", { machine: machineLabel(machine), why })}
        </Notice>
      ))}
      {!list && !loaded.error && <Notice tone="hint">{t("Loading…")}</Notice>}
      <div className="trigger-list">
        {shown.map(({ rule, i }) =>
          open === i ? (
            <TriggerForm
              key={i}
              rule={rule}
              machineId={machineId}
              dir={dir}
              workspace={workspace}
              onChange={(next) => change(i, next)}
              onDone={() => setOpen(null)}
              onRemove={() => remove(i)}
              blocked={blocked}
            />
          ) : (
            <div key={i} className={`trigger-row${rule.enabled ? "" : " is-off"}`}>
              <input type="checkbox" checked={rule.enabled} aria-label={t("On")} onChange={(e) => change(i, { ...rule, enabled: e.target.checked })} />
              <span className="trigger-row-text">
                <strong>{rule.name || t("Unnamed")}</strong>
                <span className="trigger-words">{redact(ruleWords(rule, machineLabel))}</span>
              </span>
              <button type="button" className="btn sm" disabled={open !== null && !!blocked} onClick={() => setOpen(i)}>
                {t("Edit")}
              </button>
              <Tip text={t("Remove")}>
                <button type="button" className="icon-btn danger" aria-label={t("Remove")} onClick={() => remove(i)}>
                  <Icon name="trash" size={14} />
                </button>
              </Tip>
            </div>
          ),
        )}
        {list && shown.length === 0 && <Notice tone="hint">{machineId ? t("No trigger of yours looks at this folder yet.") : t("No triggers of your own yet.")}</Notice>}
        {others > 0 && <p className="modal-hint">{tn(others, "{n} more looks only at other folders.", "{n} more look only at other folders.")}</p>}
      </div>
      <button type="button" className="btn sm" disabled={!list || rules.length >= max || !!blocked} onClick={add}>
        <Icon name="plus" size={12} /> {t("Add a trigger")}
      </button>
      {(err || loaded.error) && <Notice tone="error">{redact(err || loaded.error || "")}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !list || !!blocked}>
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}
