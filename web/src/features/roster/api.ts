// The roster: every machine with herdr's hierarchy, and what changes it — workspaces, tabs and worktrees made and closed, machines added and named.

import { check, getJSON, sendJSON, withMachine } from "@/lib/http";

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
  review?: { changed: number; fresh: number }; // uncommitted files; fresh = moved since the reviewer looked
  tabs: Tab[];
};

// a machine's vitals, read by internal/health a few minutes apart
export type Health = {
  at: string;
  os?: string;
  claude?: string;
  claude_via?: "brew" | "npm" | "native" | "";
  codex?: string;
  codex_via?: "brew" | "npm" | "native" | "";
  gemini?: string;
  gemini_via?: "brew" | "npm" | "native" | "";
  herdr?: string;
  herdr_via?: "brew" | "installer" | "";
  herdr_server?: string; // the version the running server is on, when one runs
  arch?: string;
  remuda?: string;
  remuda_running?: boolean; // and it answers on the machine's own port: terminals there go through it
  disk_free_gb: number;
  disk_used_pct: number;
  load1: number;
  uptime?: string;
  outdated_hooks?: { agent: string; have?: string; want?: string }[]; // herdr's agent integrations behind the installed herdr
  plugins?: { id: string; source: string }[]; // herdr plugins installed from GitHub, the ones `herdr plugin update` refreshes
  error?: string;
};

// the newest versions out there; remuda is its own newest release on GitHub
export type Latest = { at: string; claude?: string; codex?: string; gemini?: string; herdr?: string; remuda?: string };

export type Tailscale = { state: "starting" | "needs_login" | "running" | "error"; login_url?: string; url?: string; hostname?: string; error?: string };

export type Machine = {
  id: string;
  label: string;
  host?: string;
  source?: "herdr";
  state: "connected" | "connecting" | "error";
  error?: string;
  sessions: Session[];
  health?: Health;
};

export type AgentKind = { id: string; label: string };

export type Roster = { machines: Machine[]; agents: AgentKind[]; version?: string; latest?: Latest; tailscale?: Tailscale };

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
    warning: body.agent_error ? `Created, but the agent failed to start: ${body.agent_error}` : undefined,
  };
}

export const createSession = (req: { machine: string; name: string; dir: string; agent: string }) => create("/api/sessions", req);

export const createTab = (req: { machine: string; workspace: string; name: string; agent: string }) => create("/api/tabs", req);

export const createWorktree = (req: { machine: string; workspace: string; branch: string; base: string; name: string; agent: string }) => create("/api/worktrees", req);

export const killSession = (machineId: string, id: string): Promise<void> => sendJSON("DELETE", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId));

export const closeTab = (machineId: string, id: string): Promise<void> => sendJSON("DELETE", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId));

export const renameTab = (machineId: string, id: string, label: string): Promise<void> => sendJSON("PATCH", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId), { label });

export const renameSession = (machineId: string, id: string, label: string): Promise<void> =>
  sendJSON("PATCH", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId), { label });

export const addMachine = (req: { label: string; host: string }): Promise<void> => sendJSON("POST", "/api/machines", req);

export const renameMachine = (id: string, label: string): Promise<void> => sendJSON("PATCH", `/api/machines/${encodeURIComponent(id)}`, { label }).then(() => undefined);

export const removeMachine = (id: string): Promise<void> => sendJSON("DELETE", `/api/machines/${encodeURIComponent(id)}`);
