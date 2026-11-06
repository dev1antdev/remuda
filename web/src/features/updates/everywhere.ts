// Update everywhere's words: the steps grouped by machine in the server's order, how a run stands, and its summary.

import type { Step, StepRun } from "./api";
import { tn, t } from "@/lib/i18n";

// byMachine groups the steps by machine, keeping the server's order: the remotes first, the page's own host last
export function byMachine<S extends Step>(steps: S[]): { machine: string; label: string; steps: S[] }[] {
  const groups: { machine: string; label: string; steps: S[] }[] = [];
  for (const step of steps) {
    const last = groups[groups.length - 1];
    if (last && last.machine === step.machine) last.steps.push(step);
    else groups.push({ machine: step.machine, label: step.label, steps: [step] });
  }
  return groups;
}

// stateWords is a step's state as the dialog says it
export function stateWords(step: StepRun): string {
  switch (step.state) {
    case "waiting":
      return t("waiting");
    case "running":
      return t("running…");
    case "ok":
      return t("done");
    case "skipped":
      return step.error ? t("skipped: {why}", { why: step.error }) : t("skipped");
    default:
      return step.error ? t("failed: {why}", { why: step.error }) : t("failed");
  }
}

// summaryWords sums a finished run up: "7 updated, 1 failed"; while it goes, how far it is
export function summaryWords(steps: StepRun[], done: boolean): string {
  const ok = steps.filter((s) => s.state === "ok").length;
  const failed = steps.filter((s) => s.state === "failed").length;
  const skipped = steps.filter((s) => s.state === "skipped").length;
  if (!done) return t("{done} of {total} done", { done: ok + failed + skipped, total: steps.length });
  const words = [tn(ok, "{n} updated", "{n} updated")];
  if (failed) words.push(tn(failed, "{n} failed", "{n} failed"));
  if (skipped) words.push(tn(skipped, "{n} skipped", "{n} skipped"));
  return words.join(", ");
}

// restartsHost says whether the run ends with the page's own remuda restarting: its release is the last step
export const restartsHost = (steps: Step[], host: string | undefined): boolean => !!host && steps.some((s) => s.machine === host && s.what === "remuda-release");

// hostRestarting says whether a run that cannot be read any more went down with the page's own remuda: its release step
// had started, so the server is restarting onto it (and the run, kept in its memory, is gone) — no failure to show
export const hostRestarting = (steps: StepRun[], host: string | undefined): boolean =>
  !!host && steps.some((s) => s.machine === host && s.what === "remuda-release" && (s.state === "running" || s.state === "ok"));
