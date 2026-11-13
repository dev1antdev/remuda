// The Git lens's calls: a page of a checkout's history, where it stands now, one commit's files, one file's patch, and
// git's writes on it at the person's press (internal/gitgraph, internal/gitops).

import { getJSON, sendJSON } from "@/lib/http";

export type RefKind = "branch" | "remote" | "tag" | "head";
export type Ref = { kind: RefKind; name: string; head?: boolean };

export type Commit = { hash: string; parents: string[]; author: string; email: string; time: number; refs?: Ref[]; subject: string };

export type Change = { status: string; path: string };

export type Operation = "merge" | "rebase" | "cherry-pick" | "revert";

export type GitState = {
  repo: boolean;
  root?: string;
  head?: string;
  branch?: string;
  operation?: Operation;
  fingerprint?: string;
  dirty: number;
  conflicts: number;
  changes?: Change[];
};

export type GitLog = GitState & { commits: Commit[]; more: boolean; skip: number; branches: string[]; remotes: string[]; error?: string };

export type GitFile = { status: string; path: string; from?: string; add: number; del: number; binary?: boolean };

export type CommitDetail = {
  hash: string;
  parent?: string;
  author: string;
  email: string;
  time: number;
  committer: string;
  committer_email: string;
  committed: number;
  message: string;
  files: GitFile[];
  more: number;
};

export type Scope = { all: boolean; remotes: boolean };

const where = (machineId: string, dir: string) => `machine=${encodeURIComponent(machineId)}&dir=${encodeURIComponent(dir)}`;

export const readLog = (machineId: string, dir: string, scope: Scope, skip: number, limit: number) =>
  getJSON<GitLog>(`/api/git/log?${where(machineId, dir)}&all=${scope.all ? 1 : 0}&remotes=${scope.remotes ? 1 : 0}&skip=${skip}&limit=${limit}`);

export const readState = (machineId: string, dir: string) => getJSON<GitState>(`/api/git/state?${where(machineId, dir)}`);

export const readCommit = (machineId: string, dir: string, sha: string) => getJSON<CommitDetail>(`/api/git/commit?${where(machineId, dir)}&sha=${sha}`);

export const readPatch = (machineId: string, dir: string, sha: string, file: GitFile) =>
  getJSON<{ diff: string }>(`/api/git/diff?${where(machineId, dir)}&sha=${sha}&path=${encodeURIComponent(file.path)}${file.from ? `&from=${encodeURIComponent(file.from)}` : ""}`).then((r) => r.diff);

// OpRequest is one of git's writes as the server takes it, with HEAD and the branch as the page saw them
export type OpRequest = {
  op: string;
  ref?: string;
  name?: string;
  sha?: string;
  mode?: "soft" | "mixed" | "hard";
  message?: string;
  checkout?: boolean;
  no_ff?: boolean;
  force?: boolean;
  operation?: Operation;
  head: string;
  branch: string;
};

export type OpResult = { ok: boolean; output?: string; operation?: Operation; conflicts?: number; not_merged?: boolean };

export const runOp = (machineId: string, dir: string, req: OpRequest) => sendJSON<OpResult>("POST", "/api/git/op", { machine: machineId, dir, ...req });
