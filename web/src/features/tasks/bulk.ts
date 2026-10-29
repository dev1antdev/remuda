// What one action on a selection becomes: a request per task, in order, run one after another, and the words for how it went.

import { plural } from "@/lib/text";
import type { Label, Status, Task } from "./api";
import { t } from "@/lib/i18n";

export type Action =
  | { kind: "move"; statusId: number } // one project's column
  | { kind: "move-named"; column: string } // a column by its title, in each task's own project
  | { kind: "label"; labelId: number }
  | { kind: "done" }
  | { kind: "delete" };

export type Request = { id: number } & ({ op: "patch"; fields: { status_id?: number; done?: boolean } } | { op: "label"; labelId: number } | { op: "delete" });

export type Plan = { requests: Request[]; skipped: number }; // skipped: the tasks the action does not apply to

// a column a selection can move to: one project's own status, or a title the tasks' projects share
export type ColumnChoice = { title: string; statusId?: number; have: number };

const fold = (s: string) => s.trim().toLowerCase();

// plan lays the action out as requests: a task already where the action puts it is skipped, so is one whose project has no such column
export function plan(action: Action, tasks: Task[], columns: (t: Task) => Status[]): Plan {
  const requests: Request[] = [];
  let skipped = 0;
  for (const t of tasks) {
    switch (action.kind) {
      case "move":
        if (t.status_id === action.statusId) skipped++;
        else requests.push({ id: t.id, op: "patch", fields: { status_id: action.statusId } });
        break;
      case "move-named": {
        const to = columns(t).find((s) => fold(s.title) === fold(action.column));
        if (!to || to.id === t.status_id) skipped++;
        else requests.push({ id: t.id, op: "patch", fields: { status_id: to.id } });
        break;
      }
      case "label":
        if ((t.labels ?? []).some((l) => l.id === action.labelId)) skipped++;
        else requests.push({ id: t.id, op: "label", labelId: action.labelId });
        break;
      case "done":
        if (t.done) skipped++;
        else requests.push({ id: t.id, op: "patch", fields: { done: true } });
        break;
      case "delete":
        requests.push({ id: t.id, op: "delete" });
        break;
    }
  }
  return { requests, skipped };
}

// columnChoices is what the Move menu offers: the one project's columns in order, or, across projects, every column
// title the tasks' projects have, in the order first met, with how many of the tasks could go there
export function columnChoices(tasks: Task[], columns: (t: Task) => Status[]): ColumnChoice[] {
  const projects = new Set(tasks.map((t) => t.project_id));
  if (projects.size === 1 && tasks.length > 0) {
    return columns(tasks[0]!).map((s) => ({ title: s.title, statusId: s.id, have: tasks.length }));
  }
  const out: ColumnChoice[] = [];
  const seen = new Map<string, ColumnChoice>();
  for (const t of tasks) {
    const own = new Set<string>();
    for (const s of columns(t)) {
      const key = fold(s.title);
      if (own.has(key)) continue;
      own.add(key);
      let choice = seen.get(key);
      if (!choice) {
        choice = { title: s.title, have: 0 };
        seen.set(key, choice);
        out.push(choice);
      }
      choice.have++;
    }
  }
  return out;
}

// labelChoices is what the Add-label menu offers: every label, with how many of the tasks lack it
export function labelChoices(tasks: Task[], labels: Label[]): { label: Label; lacking: number }[] {
  return labels.map((label) => ({ label, lacking: tasks.filter((t) => !(t.labels ?? []).some((l) => l.id === label.id)).length }));
}

export type Outcome = { done: number; failed: number; error: string };

// runInSequence sends one item after another, going on past a refusal and keeping the first one's words
export async function runInSequence<T>(items: T[], send: (item: T) => Promise<unknown>): Promise<Outcome> {
  const out: Outcome = { done: 0, failed: 0, error: "" };
  for (const item of items) {
    try {
      await send(item);
      out.done++;
    } catch (e) {
      out.failed++;
      if (!out.error) out.error = e instanceof Error ? e.message : String(e);
    }
  }
  return out;
}

// liveRun says whether an agent is still on the task: starting or running
export const liveRun = (t: Task): boolean => t.run?.state === "starting" || t.run?.state === "running";

export type StartPlan = { starts: Task[]; skipped: Task[] }; // skipped: the tasks an agent already runs on

// startPlan lays a bulk start out: every selected task but the ones an agent is still on
export function startPlan(tasks: Task[]): StartPlan {
  return { starts: tasks.filter((t) => !liveRun(t)), skipped: tasks.filter(liveRun) };
}

// startWords: "Started 7 agents", "Started 6 agents, 1 refused: no such branch", "Started 2 agents, 1 skipped: #5 already runs"
export function startWords(outcome: Outcome, skipped: Task[]): string {
  const parts = [t("Started {agents}", { agents: plural(outcome.done, "agent") })];
  if (outcome.failed) parts.push(t("{n} refused", { n: outcome.failed }) + (outcome.error ? `: ${outcome.error}` : ""));
  if (skipped.length) parts.push(t("{n} skipped", { n: skipped.length }) + ": " + t("{tasks} already run", { tasks: skipped.map((x) => x.identifier).join(", ") }));
  return parts.join(", ");
}

const VERBS: Record<Action["kind"], string> = { move: "moved", "move-named": "moved", label: "labelled", done: "marked done", delete: "deleted" };

// outcomeWords: "3 moved", "2 moved, 1 failed: no such column", "1 labelled, 2 skipped"
export function outcomeWords(action: Action, outcome: Outcome, skipped: number): string {
  const parts = [`${outcome.done} ${VERBS[action.kind]}`];
  if (outcome.failed) parts.push(`${outcome.failed} failed${outcome.error ? `: ${outcome.error}` : ""}`);
  if (skipped) parts.push(t("{n} skipped", { n: skipped }));
  return parts.join(", ");
}

// deleteWords asks before the one action that cannot be undone
export function deleteWords(n: number): string {
  return t("Delete {tasks}? They are gone for good.", { tasks: plural(n, "task") });
}
