// Triggers: the person's rules on what a pane prints, kept on this remuda and replaced whole, and one tried on a
// pane's last rows.

import { getJSON, sendJSON } from "@/lib/http";

export type Match = "substring" | "regex";
export type Action = "push" | "preview" | "task" | "feed";

export type Rule = {
  id?: string;
  name: string;
  machine?: string; // "" for every machine
  dir?: string; // a folder of that machine and everything under it, "" for every folder
  match: Match;
  pattern: string;
  action: Action;
  column?: string; // a task action's column, by its title
  enabled: boolean;
};

// TriggersAnswer is the rules, why herdr refused them on a machine, and the patterns of the built-in watches
export type TriggersAnswer = { rules: Rule[]; refused: Record<string, string>; builtin: Record<string, string>; max: number };

// Tried is a rule run over a pane's last rows: the rows it matches (the last few), how many in all, a preview's port
export type Tried = { lines: string[]; count: number; port?: number };

export const fetchTriggers = (): Promise<TriggersAnswer> => getJSON("/api/triggers");
export const saveTriggers = (rules: Rule[]): Promise<TriggersAnswer> => sendJSON("PUT", "/api/triggers", rules);
export const tryTrigger = (machine: string, pane: string, rule: Rule): Promise<Tried> => sendJSON("POST", "/api/triggers/try", { machine, pane, rule });
