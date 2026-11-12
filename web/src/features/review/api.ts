// The Changes lens: what changed in a folder since the viewer last looked.

import { getJSON, sendJSON, withMachine } from "@/lib/http";
import type { Opinion } from "./opinion";

export type ReviewFile = { path: string; added: number; removed: number; untracked?: boolean; fresh: boolean };
export type ReviewCommit = { sha: string; subject: string; when: number; seen?: boolean }; // seen: a commit of work already reviewed
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
  error?: string;
};
export const fetchReview = (machineId: string, dir: string): Promise<Review> => getJSON(withMachine(`/api/review?dir=${encodeURIComponent(dir)}`, machineId));
export const fetchReviewDiff = (machineId: string, dir: string, path: string): Promise<string> =>
  getJSON<{ diff: string }>(withMachine(`/api/review/diff?dir=${encodeURIComponent(dir)}&path=${encodeURIComponent(path)}`, machineId)).then((d) => d.diff);
export const markReviewed = (machineId: string, dir: string, verdict?: Verdict): Promise<void> =>
  sendJSON("POST", "/api/review/seen", { machine: machineId, dir, ...(verdict ? { verdict } : {}) });

// the second opinion: another agent asked to review the folder in a tab of its own, its comments on the diff's lines
// deny_prompts: a Claude Code reviewer denies what would wait on a permission prompt, since a review only reads
export type OpinionAsk = { machine: string; pane: string; dir: string; agent: string; model?: string; close: boolean; deny_prompts: boolean };
export type OpinionAsked = { opinion: Opinion; tab: string; pane: string; agent_error?: string };
export const askOpinion = (req: OpinionAsk): Promise<OpinionAsked> => sendJSON("POST", "/api/opinion", req);
export const fetchOpinion = (machineId: string, dir: string): Promise<Opinion | null> =>
  getJSON<{ opinion: Opinion | null }>(withMachine(`/api/opinion?dir=${encodeURIComponent(dir)}`, machineId)).then((d) => d.opinion);
export const dismissOpinion = (machineId: string, dir: string, ids: number[]): Promise<Opinion> =>
  sendJSON<{ opinion: Opinion }>("POST", "/api/opinion/dismiss", { machine: machineId, dir, ids }).then((d) => d.opinion);
