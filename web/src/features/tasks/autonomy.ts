// A project's agents working on their own: where they work, the queue that starts the cards put in its Queued column,
// the pipeline's stages after the first agent's work — the settings' defaults and the words the board, a card and a
// task's run say about them. The server decides (internal/tasks: automation.go, queue.go, pipeline.go); its reasons
// come as its own English words.

import type { Machine } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { basename } from "@/lib/text";
import type { Automation, Project, Run, Stage, Status } from "./api";
import { nextWords, noRoomWords } from "./fleet";

export const MAX_STAGES = 8;
export const MAX_QUEUED = 10;

export const NO_AUTOMATION: Automation = { machine: "", dir: "", agent: "", model: "", args: "", queue: false, max: 1, stages: [], deny_prompts: true };

// automationOf is a project's settings, the defaults for one from before them; the deny switch is what the server says
// (on for settings never saved), off for settings saved before it
export const automationOf = (p?: Project): Automation => ({
  ...NO_AUTOMATION,
  ...(p?.automation ?? {}),
  stages: p?.automation?.stages ?? [],
  deny_prompts: p?.automation ? p.automation.deny_prompts === true : true,
});

// denyCovers is every agent kind the deny switch speaks for: the project's agent and each stage's
export const denyCovers = (a: Automation): string[] => [a.agent || "claude", ...a.stages.map((s) => s.agent || "claude")];

// queueColumn is the project's Queued column, if it marks one
export const queueColumn = (statuses: Status[]): Status | undefined => statuses.find((s) => s.on_queue);

// queueWords is the line under the Queued column's head: off, nothing queued, why the next card waits, what just started
export function queueWords(p: Project): { text: string; tone: "dim" | "amber" | "green" } {
  const a = automationOf(p);
  if (!a.queue) return { text: t("Starts nothing by itself: switch the queue on in the project's settings"), tone: "dim" };
  const note = p.queue;
  if (!note) return { text: t("The queue looks here every half minute"), tone: "dim" };
  const slots = t("{working} of {max} working", { working: note.working, max: note.max });
  if (note.state === "idle") return { text: a.anywhere ? t("Starts the next card put here on the machine with room · {slots}", { slots }) : t("Starts the next card put here · {slots}", { slots }), tone: "dim" };
  const fleet = note.fleet;
  const agent = a.agent || "claude";
  if (note.state === "started") {
    const where = fleet?.pick ? t("Just started {task} on {machine} · {slots}", { task: note.next || "", machine: fleet.pick.label, slots }) : t("Just started {task} · {slots}", { task: note.next || "", slots });
    return { text: where, tone: "green" };
  }
  const slotsTaken = (note.reason ?? "").startsWith("every slot is taken");
  const why = fleet && !fleet.pick && !slotsTaken ? noRoomWords(fleet) : (note.reason ?? "");
  const text = note.next ? t("{task} waits: {why}", { task: note.next, why }) : t("Waits: {why}", { why });
  const next = nextWords(fleet, agent);
  return { text: next ? `${text} · ${next}` : text, tone: slotsTaken ? "dim" : "amber" };
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
    out.push({ value: s.dir, label: basename(s.dir), hint: s.dir });
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
  anywhere: !!a.anywhere,
  agent: a.agent || "",
  model: a.model || "",
  args: a.args || "",
  queue: a.queue,
  max: a.max || 1,
  deny_prompts: !!a.deny_prompts,
  stages: a.stages.map((s) => ({ name: s.name, agent: s.agent || "", model: s.model || "", prompt: s.prompt, status_id: s.status_id || 0, pr: !!s.pr })),
});

// whyNotQueue is what the queue's switch still needs before it can be turned on, "" when nothing
export function whyNotQueue(a: Automation, statuses: Status[]): string {
  if (!a.machine || !a.dir) return t("Say where agents work first: a machine and a repository");
  if (!queueColumn(statuses)) return t("Mark a column Queued first");
  return "";
}

// whyNotSave is what the server would refuse in the agents' settings, "" when nothing: the columns are saved before
// them, so a refusal there would leave the edit half saved — or, for a queue whose column went, saved with the queue on
// and nothing for it to start
export function whyNotSave(a: Automation, statuses: Status[]): string {
  if (a.queue && (!a.machine || !a.dir)) return t("The queue needs a machine and a repository, or switch it off");
  if (a.queue && !queueColumn(statuses)) return t("The queue needs a column marked Queued, or switch it off");
  const at = a.stages.findIndex((s) => !s.pr && !s.prompt.trim());
  if (at >= 0) return t("Stage {n} needs a prompt", { n: at + 1 });
  return "";
}
