// A new arena: one prompt — a task's, or written here — and two to six lanes, each an agent and its model on a
// repository of a machine; every lane starts in a fresh worktree, and the Arenas panel shows them side by side.

import { useState } from "react";
import { actions } from "@/app/store";
import { machineOptions } from "@/features/roster/options";
import { CUSTOM, modelChoices, modelPlaceholder } from "@/features/roster/startOptions";
import { useRoster } from "@/features/roster/useRoster";
import { Dialog, Icon, Logo, Notice, Select, Tip, useSubmit } from "@/ui";
import { t } from "@/lib/i18n";
import { startArena } from "./api";
import { MAX_LANES, defaultLanes, repositories, requestOf, whyNot, type LaneDraft } from "./arena";

export default function ArenaDialog({ taskId, title, prompt: initial, at, onClose }: {
  taskId?: number;
  title?: string; // the task's identifier and title, when it is a task's arena
  prompt?: string;
  at?: { machineId: string; workspaceId: string }; // the repository on screen, the lanes' first guess
  onClose: () => void;
}) {
  const { machines, agents, refresh } = useRoster();
  const [prompt, setPrompt] = useState(initial ?? "");
  const [lanes, setLanes] = useState<LaneDraft[]>(() => defaultLanes(machines, agents, at));
  const connected = machines.filter((m) => m.state === "connected" && repositories(machines, m.id).length > 0);
  const kinds = agents.filter((a) => a.id !== "shell");
  const problem = whyNot(prompt, lanes);
  const change = (i: number, patch: Partial<LaneDraft>) => setLanes((ls) => ls.map((l, j) => (j === i ? { ...l, ...patch } : l)));
  const { busy, err, run } = useSubmit(async () => {
    await startArena({ prompt: prompt.trim(), task_id: taskId, lanes: requestOf(lanes) });
    refresh();
    onClose();
    actions.openPanel("arena");
  });
  return (
    <Dialog
      title={title ? t("Arena for {task}", { task: title }) : t("New arena")}
      icon="columns"
      size="lg"
      as="form"
      className="arena-new"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (!problem && !busy) void run();
      }}
    >
      <p className="modal-hint">
        {t("Every agent gets the same prompt in a worktree of its own, branched off the repository you pick, so their work never meets. Compare them in the Arenas panel and keep the best; the rest are closed with their work saved as patches.")}
      </p>
      <label>
        {t("Prompt")}
        <textarea rows={5} value={prompt} autoFocus={!initial} onChange={(e) => setPrompt(e.target.value)} placeholder={t("What every agent should do")} />
      </label>
      <div className="arena-lanes">
        {lanes.map((lane, i) => {
          const models = modelChoices(lane.agent);
          const repos = repositories(machines, lane.machine).map((r) => ({ ...r, icon: <Icon name="folder" size={14} /> }));
          return (
            <div key={i} className="arena-lane-row">
              <span className="arena-lane-n">{i + 1}</span>
              <Select value={lane.agent} onChange={(agent) => change(i, { agent, choice: "", custom: "" })} options={kinds.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }))} />
              {models.length > 0 ? (
                <Select value={lane.choice} onChange={(choice) => change(i, { choice })} options={models} />
              ) : (
                <input className="sm" value={lane.custom} placeholder={modelPlaceholder(lane.agent) || t("default model")} spellCheck={false} onChange={(e) => change(i, { choice: e.target.value ? CUSTOM : "", custom: e.target.value })} />
              )}
              {models.length > 0 && lane.choice === CUSTOM && <input className="sm" value={lane.custom} placeholder={modelPlaceholder(lane.agent)} spellCheck={false} onChange={(e) => change(i, { custom: e.target.value })} />}
              {connected.length > 1 && (
                <Select
                  value={lane.machine}
                  onChange={(machine) => change(i, { machine, workspace: repositories(machines, machine)[0]?.value ?? "" })}
                  options={machineOptions(connected, { onlyConnected: false })}
                />
              )}
              <Select value={lane.workspace} onChange={(workspace) => change(i, { workspace })} placeholder={t("Pick a repository")} options={repos} />
              <Tip text={t("Remove this lane")}>
                <button type="button" className="icon-btn sm" aria-label={t("Remove this lane")} disabled={lanes.length <= 2} onClick={() => setLanes((ls) => ls.filter((_, j) => j !== i))}>
                  <Icon name="x" size={14} />
                </button>
              </Tip>
            </div>
          );
        })}
      </div>
      <button type="button" className="btn sm ghost" disabled={lanes.length >= MAX_LANES} onClick={() => setLanes((ls) => [...ls, { ...ls[ls.length - 1]! }])}>
        <Icon name="plus" size={14} /> {t("Another agent")}
      </button>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        {problem && <span className="arena-why">{problem}</span>}
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !!problem}>
          {busy ? t("Starting…") : t("Start {n} agents", { n: lanes.length })}
        </button>
      </div>
    </Dialog>
  );
}
