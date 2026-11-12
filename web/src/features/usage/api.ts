// Usage: what the agents cost over a period, per machine; budgets: the limits on it.

import { getJSON, sendJSON } from "@/lib/http";

export type Period = "day" | "week" | "month";

// Budget is one limit: its scope (every machine, a machine, a folder on it), the sum and the period
export type Budget = {
  id: string;
  machine?: string; // "" or absent = every machine
  dir?: string; // only with a machine
  limit: number; // USD, API-equivalent
  period: Period;
  note?: string;
};

// BudgetStatus is a budget with what its period has cost so far; collected is absent while the spend is not read yet,
// missing names the machines of its scope that gave no report (not counted)
export type BudgetStatus = Budget & { spent: number; start: string; collected?: string; missing?: string[] };

export const fetchBudgets = (): Promise<BudgetStatus[]> => getJSON("/api/budgets");

// the list is replaced whole; the answer is every budget with its spend
export const putBudgets = (list: Budget[]): Promise<BudgetStatus[]> => sendJSON("PUT", "/api/budgets", list);

export type UsageTotals = { input: number; output: number; cache_read: number; cache_write: number; cache_write_1h?: number; cost: number; turns: number; unknown: number };
export type UsageReport = {
  since: string;
  total: UsageTotals;
  days: (UsageTotals & { date: string })[];
  projects: (UsageTotals & { project: string; dir: string; sessions: number })[];
  models: (UsageTotals & { model: string; name: string })[];
  sessions: (UsageTotals & { session: string; project: string; dir: string; model: string; first: string; last: string })[];
  files: number;
};
export type UsageMachine = { id: string; label: string; report?: UsageReport; error?: string };
export const fetchUsage = (days: number, machine: string, tz: string): Promise<{ since: string; days: number; machines: UsageMachine[] }> => {
  const p = new URLSearchParams({ days: String(days), tz });
  if (machine) p.set("machine", machine);
  return getJSON(`/api/usage?${p}`);
};

// bookContinue keeps the person's word to say "continue" to an agent a plan limit stopped, at the moment given
export const bookContinue = (machine: string, pane: string, at: string): Promise<{ continue_at: string }> => sendJSON("POST", "/api/limits/continue", { machine, pane, at });

// cancelContinue calls such a booking off
export const cancelContinue = (machine: string, pane: string): Promise<unknown> => sendJSON("DELETE", "/api/limits/continue", { machine, pane });

// Share is a count out of a whole; rate is there once the whole reaches the card's min_sample
export type Share = { count: number; of: number; rate?: number };
// Mean is an average over the runs that measured it; few under the card's min_sample: shown, never compared
export type Mean = { value?: number; n: number; few?: boolean };
export type TryLane = { agent: string; model?: string; model_id?: string };
// ScoreTry is a past task to hand to an arena: the model that ran it first, then the row's own
export type ScoreTry = { task_id: number; identifier: string; title: string; project_id: number; machine: string; dir: string; ran: string; lanes: TryLane[] };
// ScoreRow is one agent kind and model over the period (internal/scorecard)
export type ScoreRow = {
  agent: string;
  model: string; // "" when the meter never read one
  model_id?: string;
  runs: number;
  under_way?: number; // runs whose agent has not answered yet: counted in runs, not in the shares or means
  finished: Share;
  first_approved: Share;
  rejected: number;
  approved: number;
  merged: Share;
  kept: Share;
  cost_per_run: Mean;
  cost_per_approved: Mean;
  minutes_per_run: Mean;
  last: string;
  try?: ScoreTry;
};
export type Scorecard = { since: string; days: number; min_sample: number; rows: ScoreRow[]; verdicts: number; unplaced: number };

export const fetchScorecard = (days: number, tz: string): Promise<Scorecard> => getJSON(`/api/scorecard?${new URLSearchParams({ days: String(days), tz })}`);
