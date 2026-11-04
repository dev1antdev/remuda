// Moving an agent's work to another machine: what the work is and where it could land, the move started as a job,
// and where the agent carrying on ended up.

import { getJSON, sendJSON } from "@/lib/http";

export type Checkout = { dir: string; branch: string; remote: string; same: boolean }; // same: holds the source's repository
export type Target = { machine: string; label: string; checkouts: Checkout[]; error?: string };
export type Source = { root: string; branch: string; head: string; remote: string; ahead: number; staged: number; unstaged: number; untracked: number };
export type Targets = { source: Source; words: string; targets: Target[] };
export type Moved = { machine: string; pane?: string; dir?: string; branch?: string };

export const fetchTargets = (machine: string, pane: string): Promise<Targets> =>
  getJSON(`/api/teleport/targets?machine=${encodeURIComponent(machine)}&pane=${encodeURIComponent(pane)}`);

export const startMove = (req: { machine: string; pane: string; target: string; folder: string; agent: string; turns: number }): Promise<{ job: string }> =>
  sendJSON("POST", "/api/teleport", req);

export const fetchMoved = (job: string): Promise<Moved> => getJSON(`/api/teleport/${encodeURIComponent(job)}`);
