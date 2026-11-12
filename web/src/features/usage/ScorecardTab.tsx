// The Usage panel's Models tab: which model does the work best, from remuda's own records — the agents started on
// tasks, the verdicts given on their worktrees, the pull requests they opened, the arenas decided — a row per agent
// and model, the best of each column marked quietly, and a past task to try a model on in an arena.

import { useState } from "react";
import { actions } from "@/app/store";
import { repositoryOf } from "@/features/arena/arena";
import type { Machine } from "@/features/roster/api";
import { getTask, listProjects } from "@/features/tasks/api";
import { taskPrompt } from "@/features/tasks/prompt";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { ago } from "@/lib/time";
import { t, tn } from "@/lib/i18n";
import { Icon, Logo, Notice, Tip } from "@/ui";
import { fetchScorecard, type ScoreRow, type ScoreTry, type Share } from "./api";
import { agentLabel, bestByColumn, costWords, fewWords, minutesWords, modelName, reviewWords, rowKey, rowName, shareWords, tryLanes, tryWords, type Column } from "./scorecard";

// openTry fetches the past task and opens the arena on it with the two models side by side
async function openTry(pick: ScoreTry, machines: Machine[]) {
  const [task, projects] = await Promise.all([getTask(pick.task_id), listProjects()]);
  actions.openDialog({
    kind: "arena",
    taskId: task.id,
    title: `${task.identifier} ${task.title}`,
    prompt: taskPrompt(task, projects.find((p) => p.id === task.project_id), false),
    at: repositoryOf(machines, pick.machine, pick.dir),
    lanes: tryLanes(pick),
  });
}

export default function ScorecardTab({ days, machines, viewOnly }: { days: number; machines: Machine[]; viewOnly: boolean }) {
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const { data, error } = useQuery(() => fetchScorecard(days, tz), [days, tz], { keep: true });
  const [trying, setTrying] = useState("");
  const rows = data?.rows ?? [];
  const minSample = data?.min_sample ?? 5;
  const columns = bestByColumn(rows);
  const tryIt = (row: ScoreRow) => {
    if (!row.try || trying) return;
    setTrying(rowKey(row));
    openTry(row.try, machines)
      .catch((e) => actions.notify(errMsg(e), { error: true }))
      .finally(() => setTrying(""));
  };
  return (
    <section className="scorecard-tab">
      <Notice tone="hint">
        {t("From remuda's own records: the agents started on tasks and in arenas, the verdicts you gave in the Changes lens on their worktrees, the pull requests they opened and whether those merged. A share becomes a percentage from {n} runs on; before that it says how many, and no best is marked.", { n: minSample })}
      </Notice>
      {error && <Notice tone="error">{error}</Notice>}
      {data && data.unplaced > 0 && (
        <Notice tone="hint">{tn(data.unplaced, "{n} verdict of the period was given on a folder no task run worked in: it counts for no model.", "{n} verdicts of the period were given on folders no task run worked in: they count for no model.")}</Notice>
      )}
      {data && rows.length === 0 && <div className="empty">{t("No agent was started on a task or in an arena in this period. Start one from a task, or run an arena, and its model shows here.")}</div>}
      {rows.length > 0 && (
        <div className="scorecard" role="table" aria-label={t("Models scorecard")}>
          <div className="score-row score-head" role="row">
            <span role="columnheader">{t("Model")}</span>
            <span role="columnheader">{t("Finished")}</span>
            <span role="columnheader">{t("Approved first time")}</span>
            <span role="columnheader">{t("PRs merged")}</span>
            <span role="columnheader">{t("Kept in arenas")}</span>
            <span role="columnheader">{t("$ per run")}</span>
            <span role="columnheader">{t("$ per approved")}</span>
            <span role="columnheader">{t("Minutes per run")}</span>
            <span role="columnheader" className="score-try" />
          </div>
          {rows.map((row) => {
            const key = rowKey(row);
            const shareCell = (label: string, share: Share, column: Column, title?: string) => <Cell label={label} words={shareWords(share)} best={columns[column].has(key)} few={share.of > 0 && share.rate === undefined} title={title ?? fewWords(share.of, minSample)} />;
            return (
              <div key={key} className="score-row" role="row">
                <span className="score-model" role="cell" title={t("{name}, last at work {ago}", { name: rowName(row), ago: ago(row.last) })}>
                  <Logo agent={row.agent} />
                  <span className="score-model-name">
                    <span className="score-model-title">{modelName(row)}</span>
                    <span className="score-runs">
                      {agentLabel(row.agent)} · {tn(row.runs, "{n} run", "{n} runs")}
                    </span>
                  </span>
                </span>
                {shareCell(t("Finished"), row.finished, "finished")}
                {shareCell(t("Approved first time"), row.first_approved, "first_approved", reviewWords(row))}
                {shareCell(t("PRs merged"), row.merged, "merged", row.merged.of === 0 ? t("Its runs opened no pull request") : tn(row.merged.of, "{n} pull request opened", "{n} pull requests opened"))}
                {shareCell(t("Kept in arenas"), row.kept, "kept", row.kept.of === 0 ? t("In no decided arena") : tn(row.kept.of, "{n} lane in decided arenas", "{n} lanes in decided arenas"))}
                <Cell label={t("$ per run")} words={costWords(row.cost_per_run)} best={columns.cost_per_run.has(key)} few={row.cost_per_run.few && row.cost_per_run.n > 0} title={row.cost_per_run.n === 0 ? t("Not priced here") : fewWords(row.cost_per_run.n, minSample)} />
                <Cell label={t("$ per approved")} words={costWords(row.cost_per_approved)} best={columns.cost_per_approved.has(key)} few={row.cost_per_approved.few && row.cost_per_approved.value !== undefined} title={t("What the reviewed runs cost, over the approved ones")} />
                <Cell label={t("Minutes per run")} words={minutesWords(row.minutes_per_run)} best={columns.minutes_per_run.has(key)} few={row.minutes_per_run.few && row.minutes_per_run.n > 0} title={fewWords(row.minutes_per_run.n, minSample)} />
                <span className="score-try" role="cell">
                  {row.try && !viewOnly && (
                    <Tip text={tryWords(row.try)}>
                      <button type="button" className="icon-btn sm" aria-label={t("Try it on a past task…")} disabled={trying !== ""} onClick={() => tryIt(row)}>
                        <Icon name="columns" size={14} />
                      </button>
                    </Tip>
                  )}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}

// Cell is one figure of a row; on a phone it carries its column's name, since the head is not drawn there
function Cell({ label, words, best, few, title }: { label: string; words: string; best?: boolean; few?: boolean; title?: string }) {
  const tone = best ? " is-best" : few ? " is-few" : "";
  return (
    <span className={`score-cell${tone}`} role="cell" title={best ? [t("The best of the models with enough runs"), title].filter(Boolean).join(" · ") : title || undefined}>
      <span className="score-label">{label}</span>
      <span className="score-figure">{words}</span>
    </span>
  );
}
