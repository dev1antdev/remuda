// The pull request panel's calls: what gh reads of GitHub on the pane's machine, and what the person writes there at a
// press. A gh that is not installed or not signed in answers a read with a problem, which the panel shows in place of
// everything.

import { getJSON, sendJSON } from "@/lib/http";

export type Problem = { kind: "missing" | "signed-out"; machine: string; command: string };

export type Card = {
  id: string;
  number: number;
  title: string;
  url: string;
  state: string; // OPEN | MERGED | CLOSED
  draft: boolean;
  updated_at: string;
  head: string;
  base: string;
  author: string;
  checks?: string; // the head commit's rollup: SUCCESS | FAILURE | ERROR | PENDING | EXPECTED
  repository: string;
};

export type Remote = { name: string; repo?: string };
export type Checkout = { branch: string; remotes: Remote[]; repo: string };
export type BranchAnswer = { checkout?: Checkout; items?: Card[]; problem?: Problem };
export type Found = { items: Card[]; cursors: Record<string, string>; total?: number; problem?: Problem };

export type Comment = { id: string; author: string; body: string; url: string; at: string; state?: string; hunk?: string };
export type Thread = { id: string; path: string; line: number; resolved: boolean; outdated: boolean; can_resolve: boolean; can_reply: boolean; comments: Comment[]; more_replies?: number };
export type Check = { name: string; workflow?: string; run?: number; job?: number; status: string; conclusion?: string; url?: string; started_at?: string; completed_at?: string };
export type Reviewer = { login: string; state: string; team?: boolean };

export type Detail = {
  id: string;
  number: number;
  title: string;
  url: string;
  body: string;
  state: string;
  draft: boolean;
  author: string;
  head: string;
  base: string;
  head_oid: string;
  changed_files: number;
  additions: number;
  deletions: number;
  review_decision: string;
  mergeable: string;
  merge_state: string;
  auto_merge: boolean;
  merge_methods: string[];
  auto_allowed: boolean;
  repository: string;
  viewer: string;
  comments: Comment[];
  reviews: Comment[];
  threads: Thread[];
  checks: Check[];
  reviewers: Reviewer[];
  reviewers_error?: string;
  cut?: boolean;
  problem?: Problem;
};

export type Step = { number: number; name: string; status: string; conclusion?: string; started_at?: string; completed_at?: string };
export type Job = { id: number; name: string; status: string; conclusion?: string; url: string; steps: Step[]; problem?: Problem };
export type Log = { text: string; cut: boolean; problem?: Problem };
export type PRFile = { path: string; previous?: string; status: string; additions: number; deletions: number; patch?: string; no_patch?: boolean };
export type Files = { files: PRFile[]; cut?: boolean; problem?: Problem };

const q = (params: Record<string, string | number | undefined>) =>
  Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== "")
    .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
    .join("&");

export const fetchBranch = (machine: string, dir: string) => getJSON<BranchAnswer>(`/api/pr/branch?${q({ machine, dir })}`);

export type SearchAsk = { text: string; scope: string; state: string; cursors?: Record<string, string> };
export const fetchSearch = (machine: string, repo: string, ask: SearchAsk) =>
  getJSON<Found>(`/api/pr/search?${q({ machine, repo, q: ask.text, scope: ask.scope, state: ask.state, more: ask.cursors ? 1 : undefined, cursors: ask.cursors ? JSON.stringify(ask.cursors) : undefined })}`);

export const fetchDetail = (machine: string, repo: string, number: number) => getJSON<Detail>(`/api/pr/detail?${q({ machine, repo, number })}`);
export const fetchJob = (machine: string, repo: string, job: number) => getJSON<Job>(`/api/pr/job?${q({ machine, repo, job })}`);
export const fetchLog = (machine: string, repo: string, job: number) => getJSON<Log>(`/api/pr/log?${q({ machine, repo, job })}`);
export const fetchFiles = (machine: string, repo: string, number: number) => getJSON<Files>(`/api/pr/files?${q({ machine, repo, number })}`);

type Target = { machine: string; repo: string; number: number };
export const postComment = (to: Target, body: string) => sendJSON("POST", "/api/pr/comment", { ...to, body });
export const postReply = (machine: string, thread: string, body: string) => sendJSON("POST", "/api/pr/reply", { machine, thread, body });
export const setResolved = (machine: string, thread: string, resolved: boolean) => sendJSON("POST", "/api/pr/resolve", { machine, thread, resolved });
export const askAgain = (to: Target, reviewer: string) => sendJSON("POST", "/api/pr/ask-again", { ...to, reviewer });

export type StateAction = "merge" | "squash" | "rebase" | "auto" | "cancel-auto" | "close" | "reopen";
export type StateChange = { action: StateAction; head?: string; id?: string; method?: string; subject?: string; body?: string };
export const changeState = (to: Target, change: StateChange) => sendJSON("POST", "/api/pr/state", { ...to, ...change });

// fixCheck hands one failed check to an agent: the server reads it with gh on the agent's machine and sends the message
export const fixCheck = (machine: string, pane: string, url: string, check: string) => sendJSON("POST", "/api/pr/failure", { machine, pane, url, check });
