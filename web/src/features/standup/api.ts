// The standup and the recaps: what every agent did over a period, what one conversation came to.

import { getJSON } from "@/lib/http";

export type Call = { name: string; hint?: string; took: number };

export type Recap = {
  prompts: number;
  answers: number;
  took: number; // seconds the answers took
  tokens: number;
  cost: number; // USD at API prices
  calls: number;
  tools: Record<string, number>;
  failed: number;
  longest?: Call;
  commands: string[];
  edited: string[];
  read: string[];
  subagents: number;
  questions: number;
  asking: boolean; // the last turn ends in a question still without an answer
  first?: string;
  last?: string;
  said?: string; // the first line of the last answer
};

export type Item = {
  machine: string;
  label: string;
  workspace: string;
  pane: string;
  dir: string;
  agent: string;
  state: "working" | "blocked" | "done" | "idle" | "unknown";
  alert?: string;
  recap: Recap;
  error?: string;
};

export type Standup = { since: string; items: Item[]; markdown: string };

export const fetchStandup = (since: string | undefined, tz: string): Promise<Standup> => {
  const p = new URLSearchParams({ tz });
  if (since) p.set("since", since);
  return getJSON(`/api/standup?${p}`);
};

// fetchRecap is one conversation's recap: a pane's (its live session, or one of its folder by id), or a transcript on its own by folder and id
export const fetchRecap = (machine: string, pane: string, session: string | null, dir: string): Promise<{ since: string; recap: Recap }> => {
  const p = new URLSearchParams({ machine });
  if (pane) p.set("pane", pane);
  else p.set("dir", dir);
  if (session) p.set("session", session);
  return getJSON(`/api/recap?${p}`);
};
