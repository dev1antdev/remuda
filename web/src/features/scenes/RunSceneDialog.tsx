// A scene's run: first what it will do — a set-up's steps, those whose machine is away or whose agent runs there already
// unticked; a tidy's actions as the server sees them now, each its own tick — then each one's outcome as it goes.

import { useMemo, useState } from "react";
import { fetchRecipes } from "@/features/recipes/api";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { useQuery } from "@/lib/query";
import { Dialog, Icon, Notice, useSubmit } from "@/ui";
import { fetchPlan, fetchRun, fetchScenes, runScene, type Run } from "./api";
import RunItems from "./RunItems";
import { actionHint, actionWords, alreadyRuns, firstPicks, runWords, stepAway, stepWhere, stepWords } from "./scenes";
import { t } from "@/lib/i18n";

export default function RunSceneDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const { machines, agents } = useRoster();
  const share = useSharing(); // under the screen-sharing mode the hidden machines and folders by their neutral names
  const scenes = useQuery(fetchScenes, []);
  const recipes = useQuery(fetchRecipes, []).data ?? [];
  const scene = scenes.data?.find((s) => s.id === id) ?? null;
  const tidy = scene?.kind === "tidy";
  const [started, setStarted] = useState<Run | null>(null);
  const plan = useQuery(() => fetchPlan(id), [id], { enabled: tidy && !started });
  const [finished, setFinished] = useState(false);
  // the run read every second while it goes, and no more once it is done
  const progress = useQuery(
    async () => {
      if (!started) return undefined;
      const read = await fetchRun(started.id);
      if (read.done) setFinished(true);
      return read;
    },
    [started?.id],
    { every: 1000, enabled: !!started && !finished },
  );
  const run = progress.data && progress.data.id === started?.id ? progress.data : started;
  const [ticked, setTicked] = useState<Set<string> | null>(null);
  const steps = scene?.steps ?? [];
  const actions = plan.data?.actions ?? [];
  const firstTicked = useMemo(() => (tidy ? new Set(actions.map((a) => a.id)) : new Set(firstPicks(steps, machines, recipes).map(String))), [tidy, actions, steps, machines, recipes]);
  const picks = ticked ?? firstTicked;
  const toggle = (key: string) => {
    const next = new Set(picks);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    setTicked(next);
  };
  const submit = useSubmit(async () => {
    const picked = tidy ? { actions: actions.filter((a) => picks.has(a.id)).map((a) => a.id) } : { steps: steps.flatMap((_, i) => (picks.has(String(i)) ? [i] : [])) };
    setStarted(await runScene(id, picked));
  });
  const count = tidy ? actions.filter((a) => picks.has(a.id)).length : steps.filter((_, i) => picks.has(String(i))).length;
  const now = Date.now();

  return (
    <Dialog
      title={scene ? t("Scene: {name}", { name: scene.name }) : t("Scene")}
      icon={tidy ? "moon" : "sun"}
      size="md"
      as="form"
      className="scene-run-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void submit.run();
      }}
    >
      {scenes.error && <Notice tone="error">{scenes.error}</Notice>}
      {scenes.loading && <Notice tone="hint">{t("Loading…")}</Notice>}
      {scenes.data && !scene && <Notice tone="error">{t("That scene is gone.")}</Notice>}
      {run ? (
        <>
          <Notice tone={run.done ? (run.items.some((i) => i.state === "refused") ? "warn" : "success") : "info"}>{run.done ? runWords(run) : t("Running, one after another…")}</Notice>
          <RunItems run={run} machines={machines} agents={agents} recipes={recipes} />
          <div className="modal-actions">
            <button type="button" className="btn primary" onClick={onClose}>
              {run.done ? t("Done") : t("Close; it goes on")}
            </button>
          </div>
        </>
      ) : scene && !tidy ? (
        <>
          <p className="modal-hint">{t("Each ticked step opens its tab — a new tab of the workspace on its folder, else a new workspace — starts what runs there and hands over its first prompt.")}</p>
          <ul className="pick-list scene-picks">
            {steps.map((step, i) => {
              const away = stepAway(step, machines);
              const there = alreadyRuns(step, machines, recipes);
              return (
                <li key={i}>
                  <label className={`pick-row${away ? " failed" : ""}`}>
                    <input type="checkbox" checked={picks.has(String(i))} disabled={away} onChange={() => toggle(String(i))} />
                    <span className="pick-name">
                      <strong>{share.redact(stepWhere(step, machines))}</strong> · {share.redact(stepWords(step, agents, recipes))}
                    </span>
                    <span className="pick-hint">{away ? t("machine away") : there ? t("runs there already") : ""}</span>
                  </label>
                </li>
              );
            })}
          </ul>
          <RunActions busy={submit.busy} count={count} label={t("Start {n}", { n: count })} err={submit.err} onClose={onClose} />
        </>
      ) : scene && tidy ? (
        <>
          {plan.loading && <Notice tone="hint">{t("Looking at every machine…")}</Notice>}
          {plan.error && <Notice tone="error">{plan.error}</Notice>}
          {(plan.data?.unread ?? []).map((u) => (
            <Notice key={u.machine + u.why} tone="warn">
              {share.redact(t("{machine}: nothing looked at — {why}", { machine: u.label, why: u.why }))}
            </Notice>
          ))}
          {plan.data && actions.length === 0 && <Notice tone="success">{t("Nothing to tidy right now.")}</Notice>}
          {actions.length > 0 && (
            <ul className="pick-list scene-picks">
              {actions.map((action) => (
                <li key={action.id}>
                  <label className="pick-row">
                    <input type="checkbox" checked={picks.has(action.id)} onChange={() => toggle(action.id)} />
                    <Icon name={action.kind === "worktree" ? "git-branch" : action.kind === "compact" ? "zap" : "x"} size={14} />
                    <span className="pick-name">{share.redact(actionWords(action, agents, now))}</span>
                    <span className="pick-hint">{share.redact(machines.length > 1 ? [machines.find((m) => m.id === action.machine)?.label, actionHint(action)].filter(Boolean).join(" · ") : actionHint(action))}</span>
                  </label>
                </li>
              ))}
            </ul>
          )}
          <RunActions busy={submit.busy} count={count} label={t("Tidy {n}", { n: count })} err={submit.err} onClose={onClose} />
        </>
      ) : null}
    </Dialog>
  );
}

// RunActions is the foot before a run: what failed to start it, Cancel, and the button with how many items it runs
function RunActions({ busy, count, label, err, onClose }: { busy: boolean; count: number; label: string; err: string; onClose: () => void }) {
  return (
    <>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || count === 0}>
          <Icon name="play" size={12} /> {label}
        </button>
      </div>
    </>
  );
}
