// The pure part of recipes: a recipe as the Options fold holds it and back, the request that starts one in a folder,
// the words a list says about one, and the prefs that remember where each was used last.

import type { AgentKind } from "@/features/roster/api";
import { choiceFor, EMPTY_DRAFT, modelChoices, modelOf, type Draft } from "@/features/roster/startOptions";
import { basename } from "@/lib/text";
import type { Recipe } from "./api";
import { t } from "@/lib/i18n";

export type StartRequest = { machine: string; dir: string; name: string; agent: string; model?: string; args?: string; prompt?: string };

// draftOf is a recipe as the Options fold holds it: the model as a choice or a typed id, the arguments and the prompt as written
export function draftOf(r: Recipe): Draft {
  return { ...EMPTY_DRAFT, ...choiceFor(r.agent, r.model ?? ""), args: r.args ?? "", prompt: r.prompt ?? "" };
}

// recipeOf is a draft saved under a name: only what says something is kept, nothing but the agent for a shell
export function recipeOf(name: string, agent: string, draft: Draft, where: { dir?: string; machine?: string } = {}): Recipe {
  const r: Recipe = { name, agent };
  if (agent !== "" && agent !== "shell") {
    const model = modelOf(agent, draft);
    if (model) r.model = model;
    if (draft.args.trim()) r.args = draft.args.trim();
    if (draft.prompt.trim()) r.prompt = draft.prompt.trim();
  }
  if (where.dir?.trim()) {
    r.dir = where.dir.trim();
    if (where.machine) r.machine = where.machine;
  }
  return r;
}

// startRequest is what the create route takes for a recipe started in a folder: the fields with something in them
export function startRequest(r: Recipe, machine: string, dir: string): StartRequest {
  const req: StartRequest = { machine, dir, name: "", agent: r.agent };
  if (r.agent === "" || r.agent === "shell") return req;
  if (r.model) req.model = r.model;
  if (r.args) req.args = r.args;
  if (r.prompt) req.prompt = r.prompt;
  return req;
}

// agentLabel is the agent's name as the roster says it; a shell is a plain shell
export function agentLabel(agents: AgentKind[], id: string): string {
  if (id === "" || id === "shell") return "Plain shell";
  return agents.find((a) => a.id === id)?.label ?? id;
}

// modelLabel is a model as the Select names it when it knows it, else the id as written
export function modelLabel(agent: string, model: string): string {
  return modelChoices(agent).find((c) => c.value === model)?.label ?? model;
}

// recipeWords is a recipe's row: the agent, its model, its arguments, its first prompt's start, the folder it belongs to
export function recipeWords(r: Recipe, agents: AgentKind[]): string {
  const parts = [agentLabel(agents, r.agent)];
  if (r.model) parts.push(modelLabel(r.agent, r.model));
  if (r.args) parts.push(r.args);
  if (r.prompt) parts.push(firstWords(r.prompt, 48));
  if (r.dir) parts.push(t("only in {folder}", { folder: basename(r.dir) }));
  return parts.join(" · ");
}

// firstWords is the first line of a text cut at a word, an ellipsis when something was left out
export function firstWords(text: string, max: number): string {
  const line = text.trim().split("\n")[0] ?? "";
  if (line.length <= max) return line;
  const cut = line.slice(0, max + 1);
  const at = cut.lastIndexOf(" ");
  return `${(at > max / 2 ? cut.slice(0, at) : cut.slice(0, max)).trimEnd()}…`;
}

// lastRecipeKey is the pref that remembers the recipe last started in a folder of a machine
export const lastRecipeKey = (machine: string, dir: string): string => `recipe.last:${machine}|${dir}`;

// recipeDirKey is the pref that remembers the folder a recipe was last started in
export const recipeDirKey = (id: string): string => `recipe.dir:${id}`;

// folderFor is where a recipe starts: its own folder, else the one it was started in last, else the one at hand
export function folderFor(r: Recipe, remembered: string, atHand: string): string {
  return r.dir || remembered || atHand;
}

// machineFor is the machine a recipe starts on: its own when it has a folder there, else the one at hand
export function machineFor(r: Recipe, atHand: string): string {
  return (r.dir && r.machine) || atHand;
}

// pickFirst is the recipe a dialog starts on: the one remembered for the folder when it still exists, else the first
export function pickFirst(list: Recipe[], remembered: string): Recipe | null {
  return list.find((r) => r.id === remembered) ?? list[0] ?? null;
}
