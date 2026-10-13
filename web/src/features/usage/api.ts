// Usage: what the agents cost over a period, per machine.

import { getJSON } from "@/lib/http";

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
