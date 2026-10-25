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
