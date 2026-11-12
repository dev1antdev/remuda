// The pure part of scenes: a fresh scene of each kind, a step's source as the editor picks it, the words the list, the
// editor and the run say about a step, a layout, a tidy and its actions, and which steps a run starts ticked.

import type { Recipe } from "@/features/recipes/api";
import { agentLabel, firstWords, modelLabel } from "@/features/recipes/recipes";
import type { AgentKind, Machine } from "@/features/roster/api";
import { sizeWords } from "@/features/roster/tidy";
import { t, tn } from "@/lib/i18n";
import { basename } from "@/lib/text";
import type { Action, Item, Layout, LayoutNode, Run, Scene, Step, Tidy } from "./api";

export const MAX_STEPS = 12;
export const DEFAULT_IDLE_HOURS = 8;
export const DEFAULT_COMPACT_AT = 70;
export const SHELL = "shell";

// newStartScene is a set-up with no steps yet
export const newStartScene = (): Scene => ({ name: t("Morning"), kind: "start", steps: [] });

// newTidyScene is an evening tidy with the safe choices on: the idle shells, the agents idle a working day, the clean leftover worktrees
export const newTidyScene = (): Scene => ({ name: t("Evening"), kind: "tidy", tidy: { shells: true, idle: true, idle_hours: DEFAULT_IDLE_HOURS, compact: false, worktrees: true } });

// freshStep is a step on a machine with Claude Code when the roster offers it, else the first agent
export const freshStep = (machine: string, agents: AgentKind[]): Step => ({ machine, dir: "", agent: agents.some((a) => a.id === "claude") ? "claude" : (agents[0]?.id ?? SHELL) });

// sourceOf is what a step runs as the source Select's value: "agent:<kind>", "recipe:<id>" or "shell"
export function sourceOf(step: Step): string {
  if (step.recipe) return `recipe:${step.recipe}`;
  if (!step.agent || step.agent === SHELL) return SHELL;
  return `agent:${step.agent}`;
}

// withSource is a step running what the Select picked: a recipe or a shell brings no model, arguments or prompt of the step's own
export function withSource(step: Step, value: string): Step {
  const { agent: _agent, recipe: _recipe, model: _model, args: _args, prompt, ...where } = step;
  if (value.startsWith("recipe:")) return { ...where, recipe: value.slice("recipe:".length) };
  if (value === SHELL) return { ...where, agent: SHELL };
  return { ...where, agent: value.slice("agent:".length), ...(prompt ? { prompt } : {}) };
}

// stepWhat is what a step runs: the agent with its model and the start of its prompt, a recipe by its name, a plain shell
export function stepWhat(step: Step, agents: AgentKind[], recipes: Recipe[]): string {
  if (step.recipe) {
    const recipe = recipes.find((r) => r.id === step.recipe);
    return recipe ? t("recipe {name}", { name: recipe.name }) : t("a recipe that is gone");
  }
  const parts = [agentLabel(agents, step.agent ?? SHELL)];
  if (step.model) parts.push(modelLabel(step.agent ?? "", step.model));
  if (step.args) parts.push(step.args);
  if (step.prompt) parts.push(firstWords(step.prompt, 40));
  return parts.join(" · ");
}

// stepWhere is where a step runs: its folder's name on its machine
export function stepWhere(step: Step, machines: Machine[]): string {
  const folder = basename(step.dir) || step.dir || t("no folder yet");
  const machine = machines.find((m) => m.id === step.machine)?.label ?? step.machine;
  return machines.length > 1 ? t("{folder} on {machine}", { folder, machine }) : folder;
}

// panesOf are a layout's panes in herdr's order
export function panesOf(node: LayoutNode | undefined): LayoutNode[] {
  if (!node) return [];
  if (node.type !== "split") return [node];
  return [...panesOf(node.first), ...panesOf(node.second)];
}

// layoutWords is what a captured layout lays out: its panes, which one the agent starts in (when one does), the commands it runs
export function layoutWords(layout: Layout, agent = true): string {
  const panes = panesOf(layout.root);
  const parts = [tn(panes.length, "{n} pane", "{n} panes")];
  if (agent && panes.length > 1) parts.push(t("the agent in pane {n}", { n: layout.agent + 1 }));
  const commands = panes.filter((p) => p.command?.length).map((p) => (p.command ?? []).join(" "));
  if (commands.length) parts.push(t("runs {commands}", { commands: commands.join(", ") }));
  return parts.join(" · ");
}

// stepWords is a step's line in the list: what runs, its tab's name, its layout
export function stepWords(step: Step, agents: AgentKind[], recipes: Recipe[]): string {
  const parts = [stepWhat(step, agents, recipes)];
  if (step.tab) parts.push(t("tab “{name}”", { name: step.tab }));
  if (step.layout) parts.push(layoutWords(step.layout, sourceOf(step) !== SHELL));
  return parts.join(" · ");
}

// tidyChoices are the things a tidy does, a line each
export function tidyChoices(tidy: Tidy): string[] {
  const out: string[] = [];
  if (tidy.compact) out.push(t("/compact idle Claude Codes past {n} % of their context", { n: tidy.compact_at ?? DEFAULT_COMPACT_AT }));
  if (tidy.idle) out.push(tn(tidy.idle_hours ?? DEFAULT_IDLE_HOURS, "Close agents idle {n} hour or more", "Close agents idle {n} hours or more"));
  if (tidy.shells) out.push(t("Close the shells sitting at their prompt"));
  if (tidy.worktrees) out.push(t("Remove the clean leftover worktrees"));
  return out;
}

// tidyWords is a tidy's line in the list: what it does and where
export function tidyWords(tidy: Tidy, machines: Machine[]): string {
  const where = tidy.machines?.length ? tidy.machines.map((id) => machines.find((m) => m.id === id)?.label ?? id).join(", ") : t("every machine");
  return [...tidyChoices(tidy), where].join(" · ");
}

// sceneWords is a scene's line in the list
export function sceneWords(scene: Scene, agents: AgentKind[], recipes: Recipe[], machines: Machine[]): string {
  if (scene.kind === "tidy") return scene.tidy ? tidyWords(scene.tidy, machines) : "";
  const steps = scene.steps ?? [];
  return [tn(steps.length, "{n} step", "{n} steps"), ...steps.slice(0, 3).map((s) => `${stepWhere(s, machines)}: ${stepWhat(s, agents, recipes)}`)].join(" · ");
}

// paneWords names a pane an action is about: the agent or a shell, in its workspace and tab
function paneWords(action: Action, agents: AgentKind[]): { who: string; where: string } {
  const who = action.agent ? agentLabel(agents, action.agent) : t("the shell");
  const tab = action.tab && !/^\d+$/.test(action.tab) ? ` · ${action.tab}` : "";
  return { who, where: `${action.workspace || action.pane || ""}${tab}` };
}

// actionWords is a tidy's action as the person judges it before it runs
export function actionWords(action: Action, agents: AgentKind[], now: number): string {
  const { who, where } = paneWords(action, agents);
  switch (action.kind) {
    case "compact":
      return t("/compact {agent} in {where}, at {n} % of its context", { agent: who, where, n: action.context ?? 0 });
    case "close-agent":
      return t("Close {agent} in {where}, idle {hours} h", { agent: who, where, hours: Math.floor((now - (action.since ?? now)) / 3_600_000) });
    case "close-shell":
      return t("Close the shell in {where}", { where });
    case "worktree":
      return t("Remove the worktree {name}", { name: worktreeName(action.path ?? "") });
  }
}

// actionHint is what goes with an action: the tab or workspace a close takes along, a resumable session, a worktree's branch and size
export function actionHint(action: Action): string {
  if (action.kind === "worktree") return [action.branch, action.kb ? sizeWords(action.kb) : ""].filter(Boolean).join(" · ");
  const parts: string[] = [];
  if (action.kind === "close-agent") parts.push(t("its session stays: Reopen a closed agent"));
  if (action.closes === "workspace") parts.push(t("its workspace closes with it"));
  else if (action.closes === "tab") parts.push(t("its tab closes with it"));
  return parts.join(" · ");
}

// worktreeName is a worktree by its repository and its own folder, the way the leftovers dialog names one
export function worktreeName(path: string): string {
  const parts = path.split("/").filter(Boolean);
  return parts.slice(-2).join("/");
}

// itemWords is how an item of a run stands
export function itemWords(item: Item): string {
  switch (item.state) {
    case "waiting":
      return t("waiting");
    case "running":
      return t("running…");
    case "started":
      return item.made_workspace ? t("started in a new workspace") : t("started in a new tab");
    case "done":
      return item.note ? t("done · {note}", { note: item.note }) : t("done");
    case "refused":
      return t("refused: {why}", { why: item.note ?? "" });
    case "away":
      return t("its machine is away: {why}", { why: item.note ?? "" });
    case "skipped":
      return t("skipped: {why}", { why: item.note ?? "" });
  }
}

// itemTone is an item's mark: a good one, a refusal, one that did not run
export const itemTone = (item: Item): "ok" | "bad" | "quiet" | "busy" =>
  item.state === "started" || item.state === "done" ? "ok" : item.state === "refused" ? "bad" : item.state === "running" ? "busy" : "quiet";

// runWords counts a run's items by how they went: "3 started, 1 away"
export function runWords(run: Run): string {
  const count = (state: Item["state"]) => run.items.filter((i) => i.state === state).length;
  const parts = [
    count("started") && t("{n} started", { n: count("started") }),
    count("done") && t("{n} done", { n: count("done") }),
    count("refused") && t("{n} refused", { n: count("refused") }),
    count("away") && t("{n} away", { n: count("away") }),
    count("skipped") && t("{n} skipped", { n: count("skipped") }),
  ].filter(Boolean);
  return parts.join(", ") || t("nothing to do");
}

// stepAway says whether a step's machine is not connected now
export const stepAway = (step: Step, machines: Machine[]): boolean => machines.find((m) => m.id === step.machine)?.state !== "connected";

// stepKind is the agent a step starts, its recipe's when it names one; "" for a shell or a recipe that is gone
export function stepKind(step: Step, recipes: Recipe[]): string {
  const agent = step.recipe ? recipes.find((r) => r.id === step.recipe)?.agent : step.agent;
  return !agent || agent === SHELL ? "" : agent;
}

// alreadyRuns says whether that agent already runs in a workspace on the step's folder: a second press of the morning
// would start another beside it
export function alreadyRuns(step: Step, machines: Machine[], recipes: Recipe[]): boolean {
  const kind = stepKind(step, recipes);
  const machine = machines.find((m) => m.id === step.machine);
  if (!kind || !machine) return false;
  return machine.sessions.some((s) => s.dir === step.dir && s.tabs.some((tab) => tab.panes.some((p) => p.agent === kind)));
}

// firstPicks are the steps a run starts ticked: every one whose machine is connected and whose agent is not there already
export const firstPicks = (steps: Step[], machines: Machine[], recipes: Recipe[]): number[] =>
  steps.flatMap((step, i) => (stepAway(step, machines) || alreadyRuns(step, machines, recipes) ? [] : [i]));

// stepsOfWorkspaces is a step for every workspace open now on a connected machine: its folder and the agent its first agent pane runs, else a shell
export function stepsOfWorkspaces(machines: Machine[]): Step[] {
  const out: Step[] = [];
  for (const m of machines) {
    if (m.state !== "connected") continue;
    for (const s of m.sessions) {
      if (!s.dir || s.scratch || out.some((step) => step.machine === m.id && step.dir === s.dir)) continue;
      const agent = s.tabs.flatMap((tab) => tab.panes).find((p) => p.agent)?.agent;
      out.push({ machine: m.id, dir: s.dir, agent: agent || SHELL });
    }
  }
  return out.slice(0, MAX_STEPS);
}

// whyNotSave is why a scene cannot be saved as it is, "" when it can
export function whyNotSave(scene: Scene): string {
  if (!scene.name.trim()) return t("A scene needs a name.");
  if (scene.kind === "tidy") {
    const tidy = scene.tidy;
    return tidy && (tidy.shells || tidy.idle || tidy.compact || tidy.worktrees) ? "" : t("Pick at least one thing to tidy.");
  }
  const steps = scene.steps ?? [];
  if (steps.length === 0) return t("Add a step.");
  if (steps.length > MAX_STEPS) return t("At most {n} steps.", { n: MAX_STEPS });
  const missing = steps.findIndex((s) => !s.dir.trim().startsWith("/"));
  return missing >= 0 ? t("Step {n} needs a folder.", { n: missing + 1 }) : "";
}

// moveStep is the steps with one moved up (-1) or down (+1); a move past either end changes nothing
export function moveStep(steps: Step[], i: number, by: -1 | 1): Step[] {
  const j = i + by;
  if (j < 0 || j >= steps.length) return steps;
  const out = [...steps];
  const [moved] = out.splice(i, 1);
  if (moved) out.splice(j, 0, moved);
  return out;
}
