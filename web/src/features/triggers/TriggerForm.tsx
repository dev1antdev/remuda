// One trigger's fields — its name, what it looks for, where, what it does — and a Test that runs it over a pane's
// last rows as herdr would.

import { useState } from "react";
import { connected } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { errMsg } from "@/lib/text";
import { Icon, Logo, Notice, Select } from "@/ui";
import { t, tn } from "@/lib/i18n";
import { tryTrigger, type Rule, type Tried } from "./api";
import { actionOptions, matchOptions, scopeOf, withScope, type Scope } from "./triggers";

export default function TriggerForm({ rule, machineId, dir, workspace, blocked, onChange, onDone, onRemove }: {
  rule: Rule;
  machineId?: string;
  dir?: string;
  workspace?: string;
  blocked: string; // why the rule cannot be saved as it is, "" when it can
  onChange: (rule: Rule) => void;
  onDone: () => void;
  onRemove: () => void;
}) {
  const { machines } = useRoster();
  const { redact } = useSharing();
  const set = (patch: Partial<Rule>) => onChange({ ...rule, ...patch });
  const live = connected(machines);
  const machine = rule.machine || machineId || live[0]?.id || "";
  const scope = scopeOf(rule);
  const machineOptions = live.map((m) => ({ value: m.id, label: redact(m.label) }));
  const scopes: { value: Scope; label: string }[] = [
    { value: "everywhere", label: t("Everywhere") },
    { value: "machine", label: t("Every folder of one machine") },
    { value: "folder", label: t("One folder and what is under it") },
  ];
  return (
    <div className="trigger-form">
      <label>
        {t("Name")}
        <input autoFocus value={rule.name} maxLength={80} placeholder={t("Deployed")} onChange={(e) => set({ name: e.target.value })} />
      </label>
      <div className="trigger-fields">
        <label>
          {t("Look for")}
          <Select value={rule.match} onChange={(match) => set({ match })} options={matchOptions()} />
        </label>
        <label className="grow">
          <span>
            {rule.match === "regex" ? t("Regular expression") : t("Text")} <small>{rule.match === "regex" ? t("one line at a time, as herdr reads it") : t("case and all")}</small>
          </span>
          <input className="trigger-pattern" value={rule.pattern} spellCheck={false} placeholder={rule.match === "regex" ? "ready in \\d+ ?ms" : "DEPLOYED"} onChange={(e) => set({ pattern: e.target.value })} />
        </label>
      </div>
      <div className="trigger-fields">
        <label>
          {t("Where")}
          <Select value={scope} onChange={(next) => onChange(withScope(rule, next, machine, dir ?? ""))} options={scopes} />
        </label>
        {scope !== "everywhere" && machineOptions.length > 0 && (
          <label>
            {t("Machine")}
            <Select value={machine} onChange={(id) => set({ machine: id })} options={machineOptions} />
          </label>
        )}
      </div>
      {scope === "folder" && (
        <label>
          {t("Folder")}
          <input className="trigger-pattern" value={rule.dir ?? ""} spellCheck={false} placeholder="/home/me/project" onChange={(e) => set({ dir: e.target.value })} />
        </label>
      )}
      <div className="trigger-fields">
        <label className="grow">
          {t("Then")}
          <Select value={rule.action} onChange={(action) => set({ action })} options={actionOptions()} />
        </label>
        {rule.action === "task" && (
          <label className="grow">
            <span>
              {t("Column")} <small>{t("by its title, in the task's own project")}</small>
            </span>
            <input value={rule.column ?? ""} placeholder="Testing" onChange={(e) => set({ column: e.target.value })} />
          </label>
        )}
      </div>
      <label className="trigger-on">
        <input type="checkbox" checked={rule.enabled} onChange={(e) => set({ enabled: e.target.checked })} /> {t("On")}
      </label>
      <TryOn rule={rule} machineId={machineId} workspace={workspace} blocked={blocked} />
      {blocked && <Notice tone="hint">{blocked}</Notice>}
      <div className="recipe-form-actions">
        <button type="button" className="btn sm" disabled={!!blocked} onClick={onDone}>
          <Icon name="check" size={12} /> {t("Done")}
        </button>
        <button type="button" className="btn sm ghost" onClick={onRemove}>
          <Icon name="trash" size={12} /> {t("Remove")}
        </button>
      </div>
    </div>
  );
}

// TryOn runs the rule over the last rows of a pane: the workspace's when the dialog was opened from one, else any
function TryOn({ rule, machineId, workspace, blocked }: { rule: Rule; machineId?: string; workspace?: string; blocked: string }) {
  const { machines } = useRoster();
  const { redact } = useSharing();
  const panes = connected(machines)
    .filter((m) => !machineId || m.id === machineId)
    .flatMap((m) => m.sessions.filter((s) => !workspace || s.id === workspace).flatMap((s) => s.tabs.flatMap((tab) => tab.panes.map((p) => ({ machine: m, session: s, pane: p })))));
  const options = panes.map(({ machine, session, pane }) => ({
    value: `${machine.id}|${pane.id}`,
    label: redact(`${session.name} · ${pane.agent ? pane.title || pane.agent : t("terminal")}`),
    hint: pane.id,
    icon: pane.agent ? <Logo agent={pane.agent} /> : undefined,
    group: machineId ? undefined : redact(machine.label),
  }));
  const [picked, setPicked] = useState("");
  const [tried, setTried] = useState<Tried | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const target = options.some((o) => o.value === picked) ? picked : (options[0]?.value ?? "");
  if (options.length === 0) return null;
  const run = () => {
    const [machine, pane] = target.split("|");
    setBusy(true);
    setErr("");
    tryTrigger(machine!, pane!, rule)
      .then(setTried, (e) => {
        setTried(null);
        setErr(errMsg(e));
      })
      .finally(() => setBusy(false));
  };
  return (
    <div className="trigger-try">
      <div className="trigger-fields">
        <label className="grow">
          <span>
            {t("Test on")} <small>{t("the pane's last 500 rows")}</small>
          </span>
          <Select value={target} onChange={setPicked} options={options} />
        </label>
        <button type="button" className="btn sm" disabled={busy || !!blocked} onClick={run}>
          {busy ? t("Testing…") : t("Test")}
        </button>
      </div>
      {err && <Notice tone="error">{redact(err)}</Notice>}
      {tried && (
        <div className="trigger-tried">
          <span className="trigger-words">
            {tried.count === 0 ? t("Nothing in its last rows matches.") : tn(tried.count, "{n} row matches", "{n} rows match", { n: tried.count })}
            {rule.action === "preview" && tried.count > 0 && (tried.port ? ` · ${t("port {port}", { port: tried.port })}` : ` · ${t("no port in the line")}`)}
          </span>
          {tried.lines.length > 0 && <pre>{redact(tried.lines.join("\n"))}</pre>}
        </div>
      )}
    </div>
  );
}
