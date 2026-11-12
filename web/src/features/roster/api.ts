// The roster: every machine with herdr's hierarchy, and what changes it — workspaces, tabs and worktrees made and closed, machines added and named.

import type { BudgetStatus } from "@/features/usage/api";
import type { Retirement, RetiringHit } from "@/features/usage/retired";
import type { Login } from "./signin";
import { check, getJSON, sendJSON, withMachine } from "@/lib/http";
import { t } from "@/lib/i18n";
import { includedWords, type Included } from "./included";
import type { Leftover, Outcome } from "./tidy";

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
  limit?: PaneLimit; // a subscription limit the agent stopped on, and the person's booking to continue it after the reset
  subagents?: { working?: number }; // how many of a Claude Code conversation's subagents work, for the zoo
  serves?: ServeOffer; // a dev server the pane announced and the previews found listening, offered as a preview
};

// ServeOffer is a dev server a pane announced: its port on the machine, the address as printed, what serves it, the
// trigger that offered it when it was one of the person's, and when
export type ServeOffer = { port: number; url?: string; label?: string; by?: string; at: string };

// PaneLimit is when the agent stopped on a plan limit and the harness's words, and the moment a booking continues it
export type PaneLimit = { hit_at?: string; text?: string; continue_at?: string };

// LimitWindow is one of an agent's subscription windows as it last reported it: claude | codex, 5h | week
export type LimitWindow = { agent: string; name: string; used: number; resets_at: string; at: string };

// MachineLimits is what a machine says of the plan limits: the windows read there, and whether Claude Code's status line is wrapped to report them
export type MachineLimits = { windows?: LimitWindow[]; wrapped?: boolean };

// PullRequest is the request the agent named and, for a GitHub one gh on the machine could read, how it stands
export type PullRequest = { url: string; label: string; status?: PRStatus };

export type PRStatus = {
  state: string; // OPEN | MERGED | CLOSED
  draft: boolean;
  mergeable: string; // MERGEABLE | CONFLICTING | UNKNOWN
  review: string; // APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED | ""
  threads?: number; // a GitLab merge request's discussion threads still open
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
  speed?: string; // what the last response ran at: "fast" in fast mode
};

export type Tab = {
  id: string;
  label: string;
  number: number;
  named: boolean;
  status: Status;
  panes: Pane[];
};

// FolderIconRef is a folder's icon as the roster carries it: an emoji, or a picture served at /api/icons/<image>
export type FolderIconRef = { emoji?: string; image?: string };

// JiraIssue is an issue a workspace's branch or agents name, as last read from the person's Jira site
export type JiraIssue = { key: string; summary: string; status: string; category: string; assignee?: string; url: string };

export type Session = {
  id: string;
  name: string;
  status: Status;
  active_tab_id: string;
  worktree?: { repo_key: string; repo_name: string; linked: boolean; checkout_path: string };
  dir?: string;
  branch?: string; // the branch the folder has checked out, a short hash when detached; none outside git
  folder?: string; // the folder the sidebar groups by: the repository's main folder for a checkout, else the workspace's
  icon?: FolderIconRef; // the folder's icon, the person's own
  review?: { changed: number; fresh: number; ready?: boolean }; // uncommitted files; fresh = moved since the reviewer looked; ready = rejected at the last look and the agent says it is ready for another
  muted_until?: string; // the folder is silenced until then: no push, no webhook, no feed row
  scratch?: boolean; // the floating terminal's own shell: the drawer shows it, the sidebar and the palette leave it out
  jira?: JiraIssue[]; // the Jira issues its branch and its agents name, once the person set a site
  tabs: Tab[];
};

// a machine's vitals, read by internal/health a few minutes apart
// one probe's load and the disk's free percentage
export type Sample = { at: string; load: number; disk_free: number; mem?: number; temp?: number }; // mem: % in use; temp: the CPU's °C

// what a Raspberry Pi's firmware held the board back for: now, and since boot when its word was readable
export type Throttle = { now: string[]; since?: string[]; word?: string };

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
  statusline?: boolean; // remuda's wrapper stands before Claude Code's status line there, keeping its plan limits
  claude_login?: Login; // when Claude Code's stored sign-in there ends, as `claude auth status` last said
  claude_model?: string; // the default model Claude Code's user settings keep there
  claude_fast?: boolean; // fast mode saved on in those settings: every session there starts in it
  model?: string; // the board or the computer: "Raspberry Pi 4 Model B Rev 1.5", "Mac16,1"
  mem_total_mb?: number;
  mem_avail_mb?: number; // what a new program could have without swapping
  swap_total_mb?: number;
  swap_used_mb?: number;
  temp_c?: number; // the CPU's, where the system tells it without sudo
  temp_crit_c?: number;
  freq_mhz?: number;
  freq_max_mhz?: number;
  throttle?: Throttle;
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
  stopped?: StoppedChat[]; // the chats a reboot stopped there: their panes are bare shells again, one Resume brings them back
  mac?: string; // the address a wake-up packet names, when the person set one
  limits?: MachineLimits; // the agents' plan limits as the machine last reported them
  retiring?: RetiringHit[]; // what starts or runs there on a model or mode being retired
};

// StoppedChat is a chat whose pane came back from a reboot as a bare shell: the agent it ran and the pane's title
export type StoppedChat = { pane: string; agent: string; title: string };

export type AgentKind = { id: string; label: string };

// ServerHost is the host of a remuda serving the page only (remuda -server): no machine of the fleet, its vitals behind the Server place
export type ServerHost = { id: string; label: string; health?: Health };

export type Roster = { machines: Machine[]; agents: AgentKind[]; version?: string; latest?: Latest; tailscale?: Tailscale; feed?: number; budgets?: BudgetStatus[]; jira?: string; server?: ServerHost; retirements?: Retirement[] }; // feed: the newest activity entry's id; budgets: the spending limits with their spend so far; jira: the Jira site, when set; retirements: the models and modes being retired

// fetchRoster returns null when the roster is unchanged (304 via ETag); viewOnly says a viewer link signed this page in.
export async function fetchRoster(etag: string | null): Promise<{ roster: Roster | null; etag: string | null; version: string | null; viewOnly: boolean }> {
  const res = await fetch("/api/roster", { headers: etag ? { "If-None-Match": etag } : {} });
  const version = res.headers.get("X-Remuda-Version");
  const viewOnly = res.headers.get("X-Remuda-Viewer") === "1";
  if (res.status === 304) return { roster: null, etag, version, viewOnly };
  await check(res);
  return { roster: (await res.json()) as Roster, etag: res.headers.get("ETag"), version, viewOnly };
}

export const refreshHealth = (machineId: string): Promise<{ health: Health | null; latest: Latest }> =>
  getJSON(`/api/machines/${encodeURIComponent(machineId)}/health`, { method: "POST" });

// the pane is on screen: a held "done" in the sidebar comes down
export const markSeen = (machine: string, pane: string): Promise<void> => sendJSON("POST", "/api/seen", { machine, pane });

export const submitPrompt = (machine: string, pane: string, text: string): Promise<void> => sendJSON("POST", "/api/submit", { machine, pane, text });

export const sendKeys = (machine: string, pane: string, keys: string[]): Promise<void> => sendJSON("POST", "/api/keys", { machine, pane, keys });

// Created is what the create calls return: the container exists either way, a warning says the agent in it failed to
// start, a note what a new worktree got from .worktreeinclude
export type Created = { pane: string; warning?: string; note?: string };

async function create(url: string, req: unknown): Promise<Created> {
  const body = await sendJSON<{ pane: string; agent_error?: string; included?: Included; include_error?: string }>("POST", url, req);
  return {
    pane: body.pane,
    warning: body.agent_error ? t("Created, but the agent failed to start: {error}", { error: body.agent_error }) : undefined,
    note: includedWords(body.included, body.include_error) || undefined,
  };
}

// StartExtras are the person's choices for the agent: a model, extra arguments as typed, a first prompt sent once it is up
export type StartExtras = { model?: string; args?: string; prompt?: string };

export const createSession = (req: { machine: string; name: string; dir: string; agent: string } & StartExtras) => create("/api/sessions", req);

// resume: a session id of the folder, the agent coming back to that conversation
export const createTab = (req: { machine: string; workspace: string; name: string; agent: string; resume?: string } & StartExtras) => create("/api/tabs", req);

export const createWorktree = (req: { machine: string; workspace: string; branch: string; base: string; name: string; agent: string } & StartExtras) => create("/api/worktrees", req);

// a workspace closed; the worktree it stood on, when herdr or remuda made it, taken down after it as far as nothing is lost
export const killSession = (machineId: string, id: string): Promise<{ worktree?: Outcome }> => sendJSON("DELETE", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId));
// the worktrees under the machine's ~/.herdr/worktrees no workspace has open, and the clean ones of them removed
export const listLeftovers = (machineId: string): Promise<{ root: string; leftovers: Leftover[] }> => getJSON(`/api/machines/${encodeURIComponent(machineId)}/leftovers`);
export const removeLeftovers = (machineId: string, paths: string[]): Promise<{ outcomes: Outcome[] }> => sendJSON("POST", `/api/machines/${encodeURIComponent(machineId)}/leftovers/remove`, { paths });

export const closeTab = (machineId: string, id: string): Promise<void> => sendJSON("DELETE", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId));

export const renameTab = (machineId: string, id: string, label: string): Promise<void> => sendJSON("PATCH", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId), { label });

export const renameSession = (machineId: string, id: string, label: string): Promise<void> =>
  sendJSON("PATCH", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId), { label });

export const addMachine = (req: { label: string; host: string }): Promise<void> => sendJSON("POST", "/api/machines", req);

export const renameMachine = (id: string, label: string): Promise<void> => sendJSON("PATCH", `/api/machines/${encodeURIComponent(id)}`, { label }).then(() => undefined);

export const removeMachine = (id: string): Promise<void> => sendJSON("DELETE", `/api/machines/${encodeURIComponent(id)}`);

// a workspace folder silenced until a moment: no push, no webhook, no feed row from its panes until then
export const muteWorkspace = (machine: string, dir: string, until: Date): Promise<{ muted_until: string }> => sendJSON("PUT", "/api/mutes", { machine, dir, until: until.toISOString() });

export const unmuteWorkspace = (machine: string, dir: string): Promise<void> => sendJSON("DELETE", "/api/mutes", { machine, dir }).then(() => undefined);

export const setAwake = (id: string, on: boolean): Promise<Awake> => sendJSON("PATCH", `/api/machines/${encodeURIComponent(id)}/awake`, { on });

// setMachineMAC keeps the address a wake-up packet names; an empty one forgets it
export const setMachineMAC = (id: string, mac: string): Promise<{ mac: string }> => sendJSON("PATCH", `/api/machines/${encodeURIComponent(id)}`, { mac });

// wakeMachine sends the magic packet from this remuda's own network; the answer says on how many addresses
export const wakeMachine = (id: string): Promise<{ sent: number; mac: string }> => sendJSON("POST", `/api/machines/${encodeURIComponent(id)}/wake`, {});

// Handed is what a hand-off to the agent answers: the request's label and how long the message was
export type Handed = { ok: boolean; label: string; chars: number };

// the failed checks' log of the pane's pull request, read with gh on the machine, sent to the pane's agent
export const handFailure = (machine: string, pane: string): Promise<Handed> => sendJSON("POST", "/api/pr/failure", { machine, pane });

// the reviews and line comments of the pane's pull request, sent to the pane's agent
export const sendReview = (machine: string, pane: string): Promise<Handed> => sendJSON("POST", "/api/pr/review", { machine, pane });

// a linked worktree's workspace closed, the worktree removed and its branch deleted on the machine
export const removeWorktree = (machineId: string, id: string): Promise<{ ok: boolean; branch: string; repo: string }> =>
  sendJSON("DELETE", withMachine(`/api/worktrees/${encodeURIComponent(id)}`, machineId));
