// What a task's agent runs spent, as words: the cost, the time a run took, the model.

import { elapsed } from "@/lib/time";
import { usd } from "@/features/usage/format";
import type { Run, Task } from "./api";

type Spent = Pick<Run, "state" | "started" | "cost" | "model" | "took">;

// runSeconds is how long a run has taken: its own figure once it finished, else the time since it started;
// nothing for a pane that went away before any finish was noted
export function runSeconds(run: Spent, now: number): number {
  if (run.took && run.took > 0) return run.took;
  if (run.state === "gone" || run.state === "failed") return 0;
  return Math.max(0, Math.round((now - new Date(run.started).getTime()) / 1000));
}

// runTime says the seconds as a span: "40 s", "38 min", "1 h 12 min"
export const runTime = (seconds: number, now: number): string => elapsed(now - seconds * 1000, now);

// runWords is the run block's line, "$4.20 · 38 min · Opus 4.5", each part only when known; "" when nothing is
export function runWords(run: Spent, now: number): string {
  const parts: string[] = [];
  if (run.cost && run.cost > 0) parts.push(usd(run.cost));
  const seconds = runSeconds(run, now);
  if (seconds > 0) parts.push(runTime(seconds, now));
  if (run.model) parts.push(run.model);
  return parts.join(" · ");
}

// taskCost is what every agent started on a task spent: the server's sum over its runs, else the latest run's figure
export const taskCost = (t: Pick<Task, "cost" | "run">): number => t.cost ?? t.run?.cost ?? 0;
