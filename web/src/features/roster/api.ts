// The roster: every machine with herdr's hierarchy, and what changes it — workspaces, tabs and worktrees made and closed, machines added and named.

import type { BudgetStatus } from "@/features/usage/api";
import { check, getJSON, sendJSON, withMachine } from "@/lib/http";
import { t } from "@/lib/i18n";

export type Status = "run" | "input" | "idle" | "done" | "shell";

export type Pane = {
  id: string;
  title: string;
  agent: string; // "" for a plain shell
  cwd: string;
  focused: boolean;
  status: Status;
  cols: number;
  rows: number;
  // the agent's lifecycle as the server reads it (last_active in unix ms); absent for a shell
  radar?: { state: "working" | "blocked" | "done" | "idle" | "unknown"; last_active?: number; since?: number; alert?: string };
  usage?: PaneUsage;
  said?: string; // the first line of the agent's last answer, once it finished
  pr?: PullRequest; // the last pull or merge request the agent named, "PR #15" / "MR !9"
};

// PullRequest is the request the agent named and, for a GitHub one gh on the machine could read, how it stands
export type PullRequest = { url: string; label: string; status?: PRStatus };

export type PRStatus = {
  state: string; // OPEN | MERGED | CLOSED
  draft: boolean;
  mergeable: string; // MERGEABLE | CONFLICTING | UNKNOWN
  review: string; // APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED | ""
  checks: { total: number; pass: number; fail: number; pending: number; skipped: number };
  at: string;
};

export type PaneUsage = {
  model: string;
  name: string;
  context: number; // tokens the last response read
  window: number; // the model's context window
  cost: number; // USD, API-equivalent
  turns: number;
  tokens: { input: number; output: number; cache_read: number; cache_write: number; cache_write_1h?: number; web_search?: number };
  updated?: number;
  unknown?: boolean;
};

export type Tab = {
  id: string;
  label: string;
  number: number;
  named: boolean;
  status: Status;
  panes: Pane[];
};

export type Session = {
  id: string;
  name: string;
  status: Status;
  active_tab_id: string;
  worktree?: { repo_key: string; repo_name: string; linked: boolean; checkout_path: string };
  dir?: string;
  review?: { changed: number; fresh: number; ready?: boolean }; // uncommitted files; fresh = moved since the reviewer looked; ready = rejected at the last look and the agent says it is ready for another
  tabs: Tab[];
};

// a machine's vitals, read by internal/health a few minutes apart
// one probe's load and the disk's free percentage
export type Sample = { at: string; load: number; disk_free: number };

export type Health = {
  at: string;
  os?: string;
  claude?: string;
  claude_via?: "brew" | "npm" | "native" | "";
  codex?: string;
  codex_via?: "brew" | "npm" | "native" | "";
  gemini?: string;
  gemini_via?: "brew" | "npm" | "native" | "";
  kimi?: string;
  kimi_via?: "brew" | "npm" | "native" | "";
  herdr?: string;
  herdr_via?: "brew" | "installer" | "";
  herdr_server?: string; // the version the running server is on, when one runs
  arch?: string;
  remuda?: string;
  remuda_running?: boolean; // and it answers on the machine's own port: terminals there go through it
  disk_free_gb: number;
  disk_used_pct: number;
  load1: number;
  cores?: number; // what a load of 1.0 per core means
  history?: Sample[]; // the day's probes, oldest first, kept in the state dir's vitals.json
  uptime?: string;
  outdated_hooks?: { agent: string; have?: string; want?: string }[]; // herdr's agent integrations behind the installed herdr
  plugins?: { id: string; source: string }[]; // herdr plugins installed from GitHub, the ones `herdr plugin update` refreshes
  error?: string;
  code_server?: string; // the release linked under ~/.local/opt/code-server
};

// the newest versions out there; remuda is its own newest release on GitHub
export type Latest = { at: string; claude?: string; codex?: string; gemini?: string; kimi?: string; herdr?: string; remuda?: string; code?: string }; // code: the code-server this remuda installs

export type Tailscale = { state: "starting" | "needs_login" | "running" | "error"; login_url?: string; url?: string; hostname?: string; error?: string };

// the keep-awake switch of a machine and whether the hold stands: how many of its agents work, or why it could not hold
export type Awake = { on: boolean; held: boolean; working: number; error?: string };

export type Machine = {
  id: string;
  label: string;
  host?: string;
  source?: "herdr";
  state: "connected" | "connecting" | "error";
  error?: string;
  sessions: Session[];
  health?: Health;
  awake?: Awake;
};

export type AgentKind = { id: string; label: string };

export type Roster = { machines: Machine[]; agents: AgentKind[]; version?: string; latest?: Latest; tailscale?: Tailscale; feed?: number; budgets?: BudgetStatus[] }; // feed: the newest activity entry's id; budgets: the spending limits with their spend so far

// fetchRoster returns null when the roster is unchanged (304 via ETag).
export async function fetchRoster(etag: string | null): Promise<{ roster: Roster | null; etag: string | null; version: string | null }> {
  const res = await fetch("/api/roster", { headers: etag ? { "If-None-Match": etag } : {} });
  const version = res.headers.get("X-Remuda-Version");
  if (res.status === 304) return { roster: null, etag, version };
  await check(res);
  return { roster: (await res.json()) as Roster, etag: res.headers.get("ETag"), version };
}

export const refreshHealth = (machineId: string): Promise<{ health: Health | null; latest: Latest }> =>
  getJSON(`/api/machines/${encodeURIComponent(machineId)}/health`, { method: "POST" });

// the pane is on screen: a held "done" in the sidebar comes down
export const markSeen = (machine: string, pane: string): Promise<void> => sendJSON("POST", "/api/seen", { machine, pane });

export const submitPrompt = (machine: string, pane: string, text: string): Promise<void> => sendJSON("POST", "/api/submit", { machine, pane, text });

export const sendKeys = (machine: string, pane: string, keys: string[]): Promise<void> => sendJSON("POST", "/api/keys", { machine, pane, keys });

// Created is what the create calls return: the container exists either way, a warning says the agent in it failed to start
export type Created = { pane: string; warning?: string };

async function create(url: string, req: unknown): Promise<Created> {
  const body = await sendJSON<{ pane: string; agent_error?: string }>("POST", url, req);
  return {
    pane: body.pane,
    warning: body.agent_error ? t("Created, but the agent failed to start: {error}", { error: body.agent_error }) : undefined,
  };
}

// StartExtras are the person's choices for the agent: a model, extra arguments as typed, a first prompt sent once it is up
export type StartExtras = { model?: string; args?: string; prompt?: string };

export const createSession = (req: { machine: string; name: string; dir: string; agent: string } & StartExtras) => create("/api/sessions", req);

// resume: a session id of the folder, the agent coming back to that conversation
export const createTab = (req: { machine: string; workspace: string; name: string; agent: string; resume?: string } & StartExtras) => create("/api/tabs", req);

export const createWorktree = (req: { machine: string; workspace: string; branch: string; base: string; name: string; agent: string } & StartExtras) => create("/api/worktrees", req);

export const killSession = (machineId: string, id: string): Promise<void> => sendJSON("DELETE", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId));

export const closeTab = (machineId: string, id: string): Promise<void> => sendJSON("DELETE", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId));

export const renameTab = (machineId: string, id: string, label: string): Promise<void> => sendJSON("PATCH", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId), { label });

export const renameSession = (machineId: string, id: string, label: string): Promise<void> =>
  sendJSON("PATCH", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId), { label });

export const addMachine = (req: { label: string; host: string }): Promise<void> => sendJSON("POST", "/api/machines", req);

export const renameMachine = (id: string, label: string): Promise<void> => sendJSON("PATCH", `/api/machines/${encodeURIComponent(id)}`, { label }).then(() => undefined);

export const removeMachine = (id: string): Promise<void> => sendJSON("DELETE", `/api/machines/${encodeURIComponent(id)}`);

export const setAwake = (id: string, on: boolean): Promise<Awake> => sendJSON("PATCH", `/api/machines/${encodeURIComponent(id)}/awake`, { on });

// Handed is what a hand-off to the agent answers: the request's label and how long the message was
export type Handed = { ok: boolean; label: string; chars: number };

// the failed checks' log of the pane's pull request, read with gh on the machine, sent to the pane's agent
export const handFailure = (machine: string, pane: string): Promise<Handed> => sendJSON("POST", "/api/pr/failure", { machine, pane });

// the reviews and line comments of the pane's pull request, sent to the pane's agent
export const sendReview = (machine: string, pane: string): Promise<Handed> => sendJSON("POST", "/api/pr/review", { machine, pane });

// a linked worktree's workspace closed, the worktree removed and its branch deleted on the machine
export const removeWorktree = (machineId: string, id: string): Promise<{ ok: boolean; branch: string; repo: string }> =>
  sendJSON("DELETE", withMachine(`/api/worktrees/${encodeURIComponent(id)}`, machineId));
