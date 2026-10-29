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

// Subagent is an agent the conversation spawned through its Agent tool, as it stands: under the call in the chat
export type Subagent = {
  id: string; // agent-<hex>, the stem of its own transcript
  tool_id: string; // the Agent call that spawned it
  description: string;
  kind?: string; // fork, Explore, general-purpose, a custom agent
  model?: string;
  branch?: string; // of a worktree of its own
  status: "working" | "quiet" | "done";
  tools: number;
  failed?: number;
  doing?: Part; // the tool call still without a result
  report?: string; // the start of the text its last turn ended with
  started?: string;
  ended?: string;
  took?: number;
};

// PlanItem is one thing the agent set out to do, from its own TodoWrite / TaskCreate / TaskUpdate calls
export type PlanItem = {
  id: string;
  text: string;
  active?: string; // what the agent says while on it
  status: "pending" | "in_progress" | "completed";
};

export type ChatResponse = { session_id?: string; subagent?: string; sessions: SessionInfo[]; turns: Turn[]; plan?: PlanItem[]; subagents?: Subagent[] };

// fetchChat returns null when unchanged (304 via ETag).
export async function fetchChat(
  machineId: string,
  paneId: string, // "" with a dir: a transcript on its own, read-only
  sessionId: string | null,
  etag: string | null,
  dir?: string,
  last?: number, // only the last N turns: a wall tile
  subagent?: string, // one subagent's own conversation, read-only
): Promise<{ body: ChatResponse; etag: string | null } | null> {
  const q = new URLSearchParams({ machine: machineId });
  if (paneId) q.set("pane", paneId);
  else if (dir) q.set("dir", dir);
  if (sessionId) q.set("session", sessionId);
  if (last) q.set("last", String(last));
  if (subagent) q.set("subagent", subagent);
  const res = await fetch(`/api/chat?${q}`, { headers: etag ? { "If-None-Match": etag } : {} });
  if (res.status === 304) return null;
  await check(res);
  return { body: (await res.json()) as ChatResponse, etag: res.headers.get("ETag") };
}

// ---- a handover: this conversation continued by another agent in a new tab of the same workspace ----

export type HandedOver = { id: string; pane: string; size: number; from: string; agent_error?: string };

// handOver starts the agent asked in a new tab of the pane's workspace with the gist of its conversation as the first prompt
export const handOver = (req: { machine: string; pane: string; session: string | null; agent: string; model?: string; args?: string; turns: number }) =>
  sendJSON<HandedOver>("POST", "/api/handover", req);

// ---- quick prompts: saved texts for the composer ----
export type QuestionOption = { n: number; label: string; picked: boolean };
// Question is the dialog on the pane's screen: its words, and its choices with the cursor on one.
export type Question = { text: string; options: QuestionOption[] };
export const fetchQuestion = (machineId: string, paneId: string): Promise<Question | null> =>
  getJSON<{ question: Question | null }>(`/api/question?${new URLSearchParams({ machine: machineId, pane: paneId })}`).then((r) => r.question);

export type QuickPrompt = { id?: string; label: string; text: string; dir?: string };
export const fetchQuick = (): Promise<QuickPrompt[]> => getJSON("/api/quick");
export const saveQuick = (list: QuickPrompt[]): Promise<QuickPrompt[]> => sendJSON("PUT", "/api/quick", list);

// ---- the composer's completion: what a slash or an @ offers in the pane's folder, read on the machine ----
export type CompleteCommand = { name: string; description?: string; source: "builtin" | "user" | "project" };
export const fetchCommands = (machineId: string, dir: string, agent: string): Promise<CompleteCommand[]> =>
  getJSON<{ commands: CompleteCommand[] | null }>(`/api/complete?${new URLSearchParams({ machine: machineId, dir, kind: "commands", agent })}`).then((r) => r.commands ?? []);
export const fetchPaths = (machineId: string, dir: string): Promise<{ paths: string[]; cut?: boolean }> =>
  getJSON<{ paths: string[] | null; cut?: boolean }>(`/api/complete?${new URLSearchParams({ machine: machineId, dir, kind: "paths" })}`).then((r) => ({ paths: r.paths ?? [], cut: r.cut }));
