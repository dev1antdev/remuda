// The chat lens: a pane's conversation as the server reads it from the session file, and the saved quick prompts.

import { check, getJSON, sendJSON } from "@/lib/http";

export type Part = {
  type: "text" | "thinking" | "tool";
  text?: string;
  tool_name?: string;
  tool_id?: string; // the tool_use id
  at?: string; // when the tool was called
  took?: number; // seconds from the call to its result, absent while it runs
  input?: unknown;
  output?: string;
  is_error?: boolean;
};

export type Turn = {
  role: "user" | "assistant";
  ts?: string;
  ids?: string[];
  parts: Part[];
  took?: number; // seconds from the prompt to the answer's last record
  tokens?: number; // input, output and cache tokens of the answer's API responses
  cost?: number; // USD at API prices
  model?: string;
};

export type SessionInfo = { id: string; mod_time: string; title?: string };

// PlanItem is one thing the agent set out to do, from its own TodoWrite / TaskCreate / TaskUpdate calls
export type PlanItem = {
  id: string;
  text: string;
  active?: string; // what the agent says while on it
  status: "pending" | "in_progress" | "completed";
};

export type ChatResponse = { session_id?: string; sessions: SessionInfo[]; turns: Turn[]; plan?: PlanItem[] };

// fetchChat returns null when unchanged (304 via ETag).
export async function fetchChat(
  machineId: string,
  paneId: string, // "" with a dir: a transcript on its own, read-only
  sessionId: string | null,
  etag: string | null,
  dir?: string,
  last?: number, // only the last N turns: a wall tile
): Promise<{ body: ChatResponse; etag: string | null } | null> {
  const q = new URLSearchParams({ machine: machineId });
  if (paneId) q.set("pane", paneId);
  else if (dir) q.set("dir", dir);
  if (sessionId) q.set("session", sessionId);
  if (last) q.set("last", String(last));
  const res = await fetch(`/api/chat?${q}`, { headers: etag ? { "If-None-Match": etag } : {} });
  if (res.status === 304) return null;
  await check(res);
  return { body: (await res.json()) as ChatResponse, etag: res.headers.get("ETag") };
}

// ---- quick prompts: saved texts for the composer ----
export type QuestionOption = { n: number; label: string; picked: boolean };
// Question is the dialog on the pane's screen: its words, and its choices with the cursor on one.
export type Question = { text: string; options: QuestionOption[] };
export const fetchQuestion = (machineId: string, paneId: string): Promise<Question | null> =>
  getJSON<{ question: Question | null }>(`/api/question?${new URLSearchParams({ machine: machineId, pane: paneId })}`).then((r) => r.question);

export type QuickPrompt = { id?: string; label: string; text: string; dir?: string };
export const fetchQuick = (): Promise<QuickPrompt[]> => getJSON("/api/quick");
export const saveQuick = (list: QuickPrompt[]): Promise<QuickPrompt[]> => sendJSON("PUT", "/api/quick", list);
