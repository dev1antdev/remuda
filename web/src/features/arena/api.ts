// The arenas on the server: one prompt to several agents side by side, each in a worktree of its own.

import { getJSON, sendJSON } from "@/lib/http";

export type LaneState = "starting" | "running" | "failed" | "kept" | "discarded";

export type Lane = {
  id: number;
  machine: string;
  workspace: string; // the repository workspace its worktree branched off
  agent: string;
  model?: string;
  branch?: string;
  dir?: string; // the worktree's folder
  workspace_id?: string; // herdr's workspace for the worktree
  pane?: string;
  started?: string;
  state: LaneState;
  note?: string; // a refusal, or what became of the lane
  patch?: string; // where its work was saved when it was closed
};

export type Stat = { added: number; removed: number; files: number };

export type Arena = {
  id: number;
  title: string;
  prompt: string;
  task_id?: number;
  created: string;
  lanes: Lane[];
  kept?: number;
  stats?: Record<string, Stat>; // by lane id, read on the lanes' machines for the newest arenas
};

export type LaneRequest = { machine: string; workspace: string; agent: string; model: string };

export const fetchArenas = (): Promise<Arena[]> => getJSON("/api/arenas");

export const startArena = (req: { prompt: string; task_id?: number; lanes: LaneRequest[] }): Promise<Arena> => sendJSON("POST", "/api/arenas", req);

// keepLane keeps one lane; every other lane's work is saved as a patch and its worktree removed
export const keepLane = (id: number, lane: number): Promise<Arena> => sendJSON("POST", `/api/arenas/${id}/keep`, { lane });

export const forgetArena = (id: number): Promise<unknown> => sendJSON("DELETE", `/api/arenas/${id}`);
