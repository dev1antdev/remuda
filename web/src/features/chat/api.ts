// The chat lens: a pane's conversation as the server reads it from the session file, and the saved quick prompts.

import { check, getJSON, sendJSON } from "@/lib/http";

export type Part = {
  type: "text" | "thinking" | "tool";
  text?: string;
  tool_name?: string;
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

export type ChatResponse = { session_id?: string; sessions: SessionInfo[]; turns: Turn[] };

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
export type QuickPrompt = { id?: string; label: string; text: string; dir?: string };
export const fetchQuick = (): Promise<QuickPrompt[]> => getJSON("/api/quick");
export const saveQuick = (list: QuickPrompt[]): Promise<QuickPrompt[]> => sendJSON("PUT", "/api/quick", list);
