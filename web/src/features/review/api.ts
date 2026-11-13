// The Changes lens: what changed in a folder since the viewer last looked, and the git writes the person presses.

import { getJSON, sendJSON, withMachine } from "@/lib/http";
import type { Opinion } from "./opinion";
import type { SecretScan, SettingsAudit } from "./secrets";

// Stat is git's line counts for one side of a file: HEAD against the index (staged), or the index against the working tree
export type Stat = { added: number; removed: number; binary?: boolean };
// ReviewFile is one uncommitted file: added and removed count HEAD against the working tree; index and work are git's
// letters for its two sides (M A D T), conflict an unmerged file's two (UU both edited, AA, DD, AU, UA, DU, UD)
export type ReviewFile = {
  path: string;
  added: number;
  removed: number;
  untracked?: boolean;
  fresh: boolean;
  index?: string;
  work?: string;
  conflict?: string;
  staged?: Stat;
  unstaged?: Stat;
};
export type ReviewCommit = { sha: string; subject: string; when: number; seen?: boolean }; // seen: a commit of work already reviewed
// OutCommit is a commit the remote does not have; Outgoing is what HEAD has over its upstream, or over the remote's default
// branch where it left it, and how many the upstream has that HEAD lacks (behind) — as of the last fetch
export type OutCommit = { sha: string; short: string; author: string; when: number; subject: string };
export type Outgoing = { against: string; upstream?: boolean; commits: OutCommit[]; total: number; behind?: number };
export type Verdict = "approved" | "changes";
export type Review = {
  repo?: string;
  branch?: string;
  head?: string;
  baseline?: { head: string; at: string; verdict?: Verdict; ready?: string }; // ready: when the agent said the rejected work is ready for another look
  commits: ReviewCommit[];
  commits_total?: number; // every commit since the last look, when more than the ones listed
  files: ReviewFile[];
  fresh: number;
  settings?: SettingsAudit[]; // Claude Code's settings files there, audited for secrets and for git tracking them
  outgoing?: Outgoing; // none without a remote to compare with
  busy?: string; // the agent at work in the checkout as the server saw it: a write would land under its feet
  error?: string;
};
export const fetchReview = (machineId: string, dir: string): Promise<Review> => getJSON(withMachine(`/api/review?dir=${encodeURIComponent(dir)}`, machineId));
// DiffFrom is which diff of a file is read: since the last look (none), one side of the index, a commit's, or from a merge base
export type DiffFrom = { side?: "staged" | "unstaged"; commit?: string; base?: string };
const diffQuery = (from: DiffFrom) => `${from.side ? `&side=${from.side}` : ""}${from.commit ? `&commit=${from.commit}` : ""}${from.base ? `&base=${from.base}` : ""}`;
export const fetchReviewDiff = (machineId: string, dir: string, path: string, from: DiffFrom = {}): Promise<string> =>
  getJSON<{ diff: string }>(withMachine(`/api/review/diff?dir=${encodeURIComponent(dir)}&path=${encodeURIComponent(path)}${diffQuery(from)}`, machineId)).then((d) => d.diff);
export const markReviewed = (machineId: string, dir: string, verdict?: Verdict): Promise<void> =>
  sendJSON("POST", "/api/review/seen", { machine: machineId, dir, ...(verdict ? { verdict } : {}) });
// the secret check of what a commit of the work would carry, and with push of the commits a push would send
export const fetchSecrets = (machineId: string, dir: string, push: boolean): Promise<SecretScan> =>
  getJSON(withMachine(`/api/review/secrets?dir=${encodeURIComponent(dir)}${push ? "&push=1" : ""}`, machineId));

// a commit opened in the lens: who, when, its message and the files it changed against its first parent
export type CommitFile = { path: string; added: number; removed: number; binary?: boolean; status: string };
export type CommitView = { sha: string; author: string; when: number; subject: string; body?: string; files: CommitFile[] };
export const fetchCommit = (machineId: string, dir: string, sha: string): Promise<CommitView> => getJSON(withMachine(`/api/review/commit?dir=${encodeURIComponent(dir)}&sha=${sha}`, machineId));

// the branch's whole work against the branch it was cut from, read only; base "" when there is nothing to compare with
export type BaseView = { base: string; detected: string; merge_base?: string; ahead: number; refs: string[]; files: ReviewFile[] };
export const fetchBase = (machineId: string, dir: string, pinned: string): Promise<BaseView> =>
  getJSON(withMachine(`/api/review/base?dir=${encodeURIComponent(dir)}${pinned ? `&base=${encodeURIComponent(pinned)}` : ""}`, machineId));

// a conflicted file: which of the index's stages it has (1 the base, 2 yours, 3 incoming) and its text with the markers
export type ConflictRead = { stages: string; text: string; truncated?: boolean; binary?: boolean; missing?: boolean };
export const fetchConflict = (machineId: string, dir: string, path: string): Promise<ConflictRead> =>
  getJSON(withMachine(`/api/review/conflict?dir=${encodeURIComponent(dir)}&path=${encodeURIComponent(path)}`, machineId));

// the writes, each at the person's press: a refusal answers 409 with why, the tool's own words a 502
export type IndexDone = { paths: string[]; skipped?: string[] };
export const stageFiles = (machineId: string, dir: string, paths: string[], bulk: boolean): Promise<IndexDone> => sendJSON("POST", "/api/review/stage", { machine: machineId, dir, paths, bulk });
export const unstageFiles = (machineId: string, dir: string, paths: string[], bulk: boolean): Promise<IndexDone> => sendJSON("POST", "/api/review/unstage", { machine: machineId, dir, paths, bulk });
export type DiscardKind = "unstaged" | "all" | "untracked";
export const discardFile = (machineId: string, dir: string, path: string, kind: DiscardKind): Promise<void> => sendJSON("POST", "/api/review/discard", { machine: machineId, dir, path, kind });
export type CommitAsk = { message: string; stage?: string[]; amend?: boolean; head?: string };
export type Committed = { sha: string; short: string; subject: string; skipped?: string[] };
export const commitWork = (machineId: string, dir: string, ask: CommitAsk): Promise<Committed> => sendJSON("POST", "/api/review/commit", { machine: machineId, dir, ...ask });
export const fetchLastCommit = (machineId: string, dir: string): Promise<{ head: string; message: string }> => getJSON(withMachine(`/api/review/last-commit?dir=${encodeURIComponent(dir)}`, machineId));

// the second opinion: another agent asked to review the folder in a tab of its own, its comments on the diff's lines
// deny_prompts: a Claude Code reviewer denies what would wait on a permission prompt, since a review only reads
export type OpinionAsk = { machine: string; pane: string; dir: string; agent: string; model?: string; close: boolean; deny_prompts: boolean };
export type OpinionAsked = { opinion: Opinion; tab: string; pane: string; agent_error?: string };
export const askOpinion = (req: OpinionAsk): Promise<OpinionAsked> => sendJSON("POST", "/api/opinion", req);
export const fetchOpinion = (machineId: string, dir: string): Promise<Opinion | null> =>
  getJSON<{ opinion: Opinion | null }>(withMachine(`/api/opinion?dir=${encodeURIComponent(dir)}`, machineId)).then((d) => d.opinion);
export const dismissOpinion = (machineId: string, dir: string, ids: number[]): Promise<Opinion> =>
  sendJSON<{ opinion: Opinion }>("POST", "/api/opinion/dismiss", { machine: machineId, dir, ids }).then((d) => d.opinion);
