// A project's agents working on their own: where they work, the queue that starts the cards put in its Queued column,
// the pipeline's stages after the first agent's work — the settings' defaults and the words the board, a card and a
// task's run say about them. The server decides (internal/tasks: automation.go, queue.go, pipeline.go); its reasons
// come as its own English words.

import type { Machine } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import type { Automation, Project, Run, Stage, Status } from "./api";

export const MAX_STAGES = 8;
export const MAX_QUEUED = 10;

export const NO_AUTOMATION: Automation = { machine: "", dir: "", agent: "", model: "", args: "", queue: false, max: 1, stages: [] };

// automationOf is a project's settings, the defaults for one from before them
export const automationOf = (p?: Project): Automation => ({ ...NO_AUTOMATION, ...(p?.automation ?? {}), stages: p?.automation?.stages ?? [] });

// queueColumn is the project's Queued column, if it marks one
export const queueColumn = (statuses: Status[]): Status | undefined => statuses.find((s) => s.on_queue);

// queueWords is the line under the Queued column's head: off, nothing queued, why the next card waits, what just started
export function queueWords(p: Project): { text: string; tone: "dim" | "amber" | "green" } {
  const a = automationOf(p);
  if (!a.queue) return { text: t("Starts nothing by itself: switch the queue on in the project's settings"), tone: "dim" };
  const note = p.queue;
  if (!note) return { text: t("The queue looks here every half minute"), tone: "dim" };
  const slots = t("{working} of {max} working", { working: note.working, max: note.max });
  if (note.state === "idle") return { text: t("Starts the next card put here · {slots}", { slots }), tone: "dim" };
  if (note.state === "started") return { text: t("Just started {task} · {slots}", { task: note.next || "", slots }), tone: "green" };
  const why = note.reason ?? "";
  return { text: note.next ? t("{task} waits: {why}", { task: note.next, why }) : t("Waits: {why}", { why }), tone: why.startsWith("every slot is taken") ? "dim" : "amber" };
}

// stageWords is a run's place in its pipeline, "" outside one
export function stageWords(run: Run, p?: Project): string {
  const pipeline = run.pipeline ?? "";
  if (!pipeline) return "";
  if (pipeline === "done") return t("Pipeline done");
  if (pipeline.startsWith("stopped: ")) return t("Pipeline stopped: {why}", { why: pipeline.slice("stopped: ".length) });
  const stages = automationOf(p).stages;
  const at = run.stage ?? 0;
  if (at === 0) return stages.length ? t("Pipeline: the first agent's work, then {stages}", { stages: stageCount(stages.length) }) : t("Pipeline: the first agent's work");
  const name = stages[at - 1]?.name ?? "";
  return t("Pipeline stage {n} of {total}{name}", { n: at, total: stages.length || at, name: name ? ` · ${name}` : "" });
}

const stageCount = (n: number) => (n === 1 ? t("1 stage") : t("{n} stages", { n }));

// repositories are the folders a machine's agents can branch worktrees off: its workspaces' own checkouts, once each
export function repositories(machines: Machine[], machineId: string): { value: string; label: string; hint: string }[] {
  const m = machines.find((x) => x.id === machineId);
  const seen = new Set<string>();
  const out: { value: string; label: string; hint: string }[] = [];
  for (const s of m?.sessions ?? []) {
    if (!s.dir || s.worktree?.linked || seen.has(s.dir)) continue;
    seen.add(s.dir);
    out.push({ value: s.dir, label: s.dir.split("/").filter(Boolean).pop() ?? s.dir, hint: s.dir });
  }
  return out;
}

export const newStage = (n: number): Stage => ({ name: t("Stage {n}", { n }), agent: "codex", model: "", prompt: "", status_id: 0, pr: false });

// saveable is the settings as the server takes them: a stage's column the edit removed, none
export function saveable(a: Automation, kept: Status[]): Automation {
  const ids = new Set(kept.filter((s) => s.id !== 0).map((s) => s.id));
  return { ...a, stages: a.stages.map((s) => ({ ...s, status_id: s.status_id && ids.has(s.status_id) ? s.status_id : 0 })) };
}

// sameAutomation says whether two settings say the same
export const sameAutomation = (a: Automation, b: Automation): boolean => JSON.stringify(normalized(a)) === JSON.stringify(normalized(b));

const normalized = (a: Automation) => ({
  machine: a.machine,
  dir: a.dir,
  agent: a.agent || "",
  model: a.model || "",
  args: a.args || "",
  queue: a.queue,
  max: a.max || 1,
  stages: a.stages.map((s) => ({ name: s.name, agent: s.agent || "", model: s.model || "", prompt: s.prompt, status_id: s.status_id || 0, pr: !!s.pr })),
});

// whyNotQueue is what the queue's switch still needs before it can be turned on, "" when nothing
export function whyNotQueue(a: Automation, statuses: Status[]): string {
  if (!a.machine || !a.dir) return t("Say where agents work first: a machine and a repository");
  if (!queueColumn(statuses)) return t("Mark a column Queued first");
  return "";
}
