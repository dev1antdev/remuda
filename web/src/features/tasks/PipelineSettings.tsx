// The project's pipeline in its settings: the stages after the first agent's work, each an agent in a new tab of the
// same worktree with its prompt and the column the card moves to, one after another as each says STAGE DONE.

import type { AgentKind } from "@/features/roster/api";
import Icon from "@/ui/icons";
import Select from "@/ui/Select";
import { Logo, Tip } from "@/ui";
import { t } from "@/lib/i18n";
import type { Stage, Status } from "./api";
import { MAX_STAGES, newStage } from "./autonomy";

export function PipelineSettings({ stages, onChange, agents, statuses }: {
  stages: Stage[];
  onChange: (stages: Stage[]) => void;
  agents: AgentKind[];
  statuses: Status[]; // the saved columns a stage can move the card to
}) {
  const set = (i: number, patch: Partial<Stage>) => onChange(stages.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const shift = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    if (j < 0 || j >= stages.length) return;
    const next = [...stages];
    [next[i], next[j]] = [next[j]!, next[i]!];
    onChange(next);
  };
  const agentChoices = agents.filter((a) => a.id !== "shell").map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }));
  const columns = [{ value: 0, label: t("stays where it is") }, ...statuses.filter((s) => s.id !== 0).map((s) => ({ value: s.id, label: s.title, icon: <span className="tasks-dot" style={{ background: `#${s.hex_color || "888"}` }} /> }))];
  return (
    <>
      <div className="tasks-field-name">{t("Pipeline")}</div>
      <p className="modal-hint">
        {t("Stages after the first agent's work, one after another: every prompt asks the agent to end with a line reading")} <code>STAGE DONE</code>
        {t(
          ", and such an answer starts the next stage — its agent in a new tab of the same worktree, the card in its column. An answer without the line — a question in its own words too — or a card you move stops it; a permission prompt or a choice only waits for you. In a prompt, {task} is the task, {previous} the last stage's answer, {branch} the branch.",
        )}
      </p>
      <div className="ps-rows">
        {stages.map((s, i) => (
          <div key={i} className="ps-stage">
            <div className="ps-row">
              <span className="badge quiet">{i + 1}</span>
              <input className="ps-name" value={s.name} placeholder={t("Stage name")} onChange={(e) => set(i, { name: e.target.value })} />
              <Select className="sm" value={s.agent || "claude"} onChange={(agent) => set(i, { agent })} options={agentChoices} />
              <input className="ps-model" value={s.model ?? ""} placeholder={t("model")} spellCheck={false} onChange={(e) => set(i, { model: e.target.value })} />
              <Select<number> className="sm" value={s.status_id ?? 0} onChange={(status_id) => set(i, { status_id })} options={columns} />
              <Tip text={t("Move up")}>
                <button type="button" className="icon-btn sm" aria-label={t("Move up")} disabled={i === 0} onClick={() => shift(i, -1)}>
                  <Icon name="chevron-up" size={14} />
                </button>
              </Tip>
              <Tip text={t("Move down")}>
                <button type="button" className="icon-btn sm" aria-label={t("Move down")} disabled={i === stages.length - 1} onClick={() => shift(i, 1)}>
                  <Icon name="chevron-down" size={14} />
                </button>
              </Tip>
              <Tip text={t("Remove stage")}>
                <button type="button" className="icon-btn sm" aria-label={t("Remove stage")} onClick={() => onChange(stages.filter((_, j) => j !== i))}>
                  <Icon name="x" size={14} />
                </button>
              </Tip>
            </div>
            <textarea
              rows={3}
              value={s.prompt}
              placeholder={s.pr ? t("Anything to add before the pull request (optional)") : t("What this stage's agent does: Review the work on {branch}. The last stage said: {previous}")}
              onChange={(e) => set(i, { prompt: e.target.value })}
            />
            <label className="tasks-toggle">
              <input type="checkbox" checked={!!s.pr} onChange={(e) => set(i, { pr: e.target.checked })} /> {t("then commit, push and open a pull request")}
            </label>
          </div>
        ))}
      </div>
      <div>
        <button type="button" className="btn sm" disabled={stages.length >= MAX_STAGES} onClick={() => onChange([...stages, newStage(stages.length + 1)])}>
          <Icon name="plus" size={12} /> {t("Stage")}
        </button>
      </div>
    </>
  );
}
