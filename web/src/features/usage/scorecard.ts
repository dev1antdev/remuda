// The models' scorecard as the page draws it: a figure's words, the best of each column among the rows with enough
// runs, a row's name and the words of its reviews; the pure part of Scorecard.tsx.

import { t, tn } from "@/lib/i18n";
import { tenth } from "@/lib/time";
import { VENDOR_NAME } from "@/ui/brand";
import type { Mean, ScoreRow, ScoreTry, Share } from "./api";
import { usd } from "./format";

// the periods the scorecard reads, in days; the pref keeps the one picked on this device
export const SCORE_PERIODS = [7, 30, 90];
export const SCORE_PERIOD_KEY = "scorecard.period";
export const DEFAULT_SCORE_PERIOD = 30;
// the Usage panel's tab, per device: the palette's "Models scorecard" writes it before it opens the panel
export const USAGE_TAB_KEY = "usage.tab";
export type UsageTab = "spend" | "models";

export type Column = "finished" | "first_approved" | "merged" | "kept" | "cost_per_run" | "cost_per_approved" | "minutes_per_run";

// HIGHER_IS_BETTER says which way a column's best lies: up for the shares, down for the money and the minutes
const HIGHER_IS_BETTER: Record<Column, boolean> = {
  finished: true,
  first_approved: true,
  merged: true,
  kept: true,
  cost_per_run: false,
  cost_per_approved: false,
  minutes_per_run: false,
};

export const rowKey = (row: Pick<ScoreRow, "agent" | "model">): string => `${row.agent}|${row.model}`;

// periodOf is a stored period, the usual one for anything else
export const periodOf = (days: number): number => (SCORE_PERIODS.includes(days) ? days : DEFAULT_SCORE_PERIOD);

// shareWords is a share as the cell says it: "80 %" once enough runs stand behind it, "2 of 3" before, "—" for none
export function shareWords(share: Share): string {
  if (share.of === 0) return "—";
  if (share.rate === undefined) return t("{count} of {of}", { count: share.count, of: share.of });
  return `${Math.round(share.rate * 100)} %`;
}

// costWords is a mean in dollars, "—" with nothing priced
export const costWords = (mean: Mean): string => (mean.value === undefined ? "—" : usd(mean.value));

// minutesWords is a mean in minutes, "—" with nothing timed
export function minutesWords(mean: Mean): string {
  if (mean.value === undefined) return "—";
  return t("{n} min", { n: mean.value < 10 ? tenth(mean.value) : Math.round(mean.value) });
}

// figureOf is the number a column compares, nothing for a figure with too few runs behind it
export function figureOf(row: ScoreRow, column: Column): number | undefined {
  const figure = row[column];
  if ("of" in figure) return figure.rate;
  return figure.few ? undefined : figure.value;
}

// bestOf are the rows holding a column's best figure, when two rows at least have one to compare; ties are all best
export function bestOf(rows: ScoreRow[], column: Column): Set<string> {
  const figures = rows.map((row) => ({ key: rowKey(row), figure: figureOf(row, column) })).filter((x): x is { key: string; figure: number } => x.figure !== undefined);
  if (figures.length < 2) return new Set();
  const pick = HIGHER_IS_BETTER[column] ? Math.max : Math.min;
  const best = pick(...figures.map((x) => x.figure));
  return new Set(figures.filter((x) => x.figure === best).map((x) => x.key));
}

// bestByColumn is bestOf for every column at once
export function bestByColumn(rows: ScoreRow[]): Record<Column, Set<string>> {
  return Object.fromEntries((Object.keys(HIGHER_IS_BETTER) as Column[]).map((column) => [column, bestOf(rows, column)])) as Record<Column, Set<string>>;
}

// modelName is a row's model, or that the meter never read one
export const modelName = (row: Pick<ScoreRow, "model">): string => row.model || t("model not read");

// agentLabel is an agent kind as a person calls it: "Claude Code"
export const agentLabel = (agent: string): string => VENDOR_NAME[agent] ?? agent;

// rowName is a row as a person calls it: "Claude Code · Opus 4.5"
export const rowName = (row: Pick<ScoreRow, "agent" | "model">): string => `${agentLabel(row.agent)} · ${modelName(row)}`;

// tryLanes are the arena's lanes a row's past task starts with: each agent and the id its --model takes
export const tryLanes = (pick: ScoreTry): { agent: string; model: string }[] => pick.lanes.map((lane) => ({ agent: lane.agent, model: lane.model_id ?? "" }));

// tryWords is what a row's "Try it on a past task" button says it will do, for its tooltip
export function tryWords(pick: ScoreTry): string {
  const names = pick.lanes.map((lane) => rowName({ agent: lane.agent, model: lane.model ?? "" }));
  return names.length > 1
    ? t("Try it on a past task: {task} in an arena, {first} beside {second}", { task: `${pick.identifier} ${pick.title}`, first: names[0]!, second: names[1]! })
    : t("Try it on a past task: {task} in an arena", { task: `${pick.identifier} ${pick.title}` });
}

// reviewWords is what the reviews of a row's runs came to, for the cell's tooltip
export function reviewWords(row: ScoreRow): string {
  const reviewed = row.first_approved.of;
  if (reviewed === 0) return t("No run of this model was reviewed in the Changes lens yet");
  const parts = [tn(row.first_approved.count, "{n} approved at the first review", "{n} approved at the first review"), tn(row.rejected, "{n} rejected before an approval", "{n} rejected before an approval")];
  const later = row.approved - row.first_approved.count;
  if (later > 0) parts.push(tn(later, "{n} of them approved later", "{n} of them approved later"));
  return parts.join(", ");
}

// fewWords says a figure stands on too few runs to compare, "" when it does not
export function fewWords(n: number, minSample: number): string {
  if (n === 0 || n >= minSample) return "";
  return tn(n, "from {n} run only", "from {n} runs only");
}
