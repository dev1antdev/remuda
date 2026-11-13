// The New worktree dialog's and the remove dialog's calls.

import { getJSON, sendJSON, withMachine } from "@/lib/http";
import { t } from "@/lib/i18n";
import type { Settings } from "../project/project";
import type { Outcome } from "../tidy";
import type { StartExtras } from "../api";
import { includedWords, type Included } from "../included";
import type { Pick } from "./plan";
import type { Lists, Source } from "./sources";

const q = encodeURIComponent;

// SourceAnswer is a repository as the dialog reads it: the source, its project settings, the machine's home and herdr's root
export type SourceAnswer = { source: Source; settings: Settings; home: string; root: string; setup: boolean };

export const fetchSource = (machine: string, dir: string): Promise<SourceAnswer> => getJSON(`/api/worktrees/source?machine=${q(machine)}&dir=${q(dir)}`);

export const fetchLists = (machine: string, repo: string): Promise<Lists> => getJSON(`/api/worktrees/github?machine=${q(machine)}&repo=${q(repo)}`);

export type Found = { kind: "issue" | "pr"; number: number; title: string; state: string; head?: string; cross_repo?: boolean };

export const lookUp = (machine: string, repo: string, number: number): Promise<Found> => getJSON(`/api/worktrees/lookup?machine=${q(machine)}&repo=${q(repo)}&number=${number}`);

export const fetchOrigin = (machine: string, root: string): Promise<void> => sendJSON("POST", "/api/worktrees/fetch", { machine, root });

export type NewWorktree = { machine: string; workspace: string; root: string; branch: string; base: string; name: string; agent: string; pick: Pick; setup: "" | "run" | "skip" } & StartExtras;

// Made is what a create answers: the pane to show, whether it opened a workspace that stood there already, how the
// setup runs, and what of it to say
export type Made = { pane: string; opened: boolean; setup?: "first" | "tab"; warning?: string; notes: string[] };

export async function createFrom(req: NewWorktree): Promise<Made> {
  const body = await sendJSON<{ pane: string; opened?: boolean; setup?: "first" | "tab"; agent_error?: string; setup_error?: string; included?: Included; include_error?: string; notes?: string[] }>("POST", "/api/worktrees", req);
  const included = includedWords(body.included, body.include_error);
  const notes = [...(body.notes ?? []), ...(body.setup_error ? [body.setup_error] : []), ...(included ? [included] : [])];
  return {
    pane: body.pane,
    opened: !!body.opened,
    setup: body.setup,
    warning: body.agent_error ? t("Created, but the agent failed to start: {error}", { error: body.agent_error }) : undefined,
    notes,
  };
}

// Removal is the remove dialog's table, read on the worktree's machine
export type Removal = {
  workspace: string;
  name: string;
  repo_name: string;
  path: string;
  branch?: string;
  repo: string;
  dirty: number;
  first?: string;
  unpushed: number;
  unmerged: number;
  merged: boolean;
  has_submodules?: boolean;
  submodules?: string[];
  made: boolean;
  shared?: string[];
  working?: string;
  merged_pr?: boolean;
  stamp: string;
  home?: string; // the machine's, for the paths said with ~
};

export const fetchRemoval = (machine: string, workspace: string): Promise<Removal> => getJSON(withMachine(`/api/worktrees/${q(workspace)}/removal`, machine));

export const removeAsShown = (machine: string, workspace: string, body: { delete_branch: boolean; stamp: string; keep_folder: boolean }): Promise<{ closed: boolean; kept?: boolean; worktree?: Outcome }> =>
  sendJSON("POST", `/api/worktrees/${q(workspace)}/remove`, { machine, ...body });
