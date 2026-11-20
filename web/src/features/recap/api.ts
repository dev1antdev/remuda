// One conversation's recap: what it came to, and when the agent worked, waited and sat idle.

import { getJSON } from "@/lib/http";

export type Call = { name: string; hint?: string; took: number };

// Span is one stretch of the agent's period: working, waiting on a question it asked, or idle; turn is the record to jump to
export type Span = { from: string; to: string; state: "working" | "waiting" | "idle"; turn?: string };

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
  spans?: Span[]; // when it worked, waited and sat idle over the period
};

// fetchRecap is one conversation's recap: a pane's (its live session, or one of its folder by id), or a transcript on its
// own by folder and id; since cuts it (RFC 3339), the whole conversation without
export const fetchRecap = (machine: string, pane: string, session: string | null, dir: string, since?: string): Promise<{ since: string; recap: Recap }> => {
  const p = new URLSearchParams({ machine });
  if (pane) p.set("pane", pane);
  else p.set("dir", dir);
  if (session) p.set("session", session);
  if (since) p.set("since", since);
  return getJSON(`/api/recap?${p}`);
};
