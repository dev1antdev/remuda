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
  // an agent's lifecycle as the server reads it: held done, held question,
  // the last moment it worked (unix ms) — absent for a shell
  radar?: { state: "working" | "blocked" | "done" | "idle" | "unknown"; last_active?: number; since?: number; alert?: string };
  // the model, how full its context is, what the session cost — once the
  // server has read the transcript
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
  dir?: string; // the workspace's folder
  review?: { changed: number; fresh: number }; // changes the reviewer has not looked at
  tabs: Tab[];
};

export type Machine = {
  id: string;
  label: string;
  host?: string;
  source?: "herdr";
  state: "connected" | "connecting" | "error";
  error?: string;
  sessions: Session[];
};

export type AgentKind = { id: string; label: string };

export type Roster = { machines: Machine[]; agents: AgentKind[] };

export type Part = {
  type: "text" | "thinking" | "tool";
  text?: string;
  tool_name?: string;
  input?: unknown;
  output?: string;
  is_error?: boolean;
};

export type Turn = { role: "user" | "assistant"; ts?: string; ids?: string[]; parts: Part[] };

export type SessionInfo = { id: string; mod_time: string; title?: string };

export type ChatResponse = { session_id?: string; sessions: SessionInfo[]; turns: Turn[] };

// check turns a failed response into an Error with the server's message; a
// 401 means the cookie is gone, and the server serves the sign-in page.
export async function check(res: Response): Promise<Response> {
  if (res.status === 401) {
    location.reload();
    throw new Error("unauthorized");
  }
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* keep statusText */
    }
    throw new Error(msg);
  }
  return res;
}

export async function sendJSON<T = unknown>(method: "POST" | "PUT" | "DELETE" | "PATCH", url: string, body?: unknown): Promise<T> {
  const res = await check(
    await fetch(url, {
      method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    }),
  );
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

function withMachine(path: string, machineId: string): string {
  return `${path}${path.includes("?") ? "&" : "?"}machine=${encodeURIComponent(machineId)}`;
}

// fetchRoster returns null when the roster is unchanged (304 via ETag).
export async function fetchRoster(etag: string | null): Promise<{ roster: Roster; etag: string | null } | null> {
  const res = await fetch("/api/roster", { headers: etag ? { "If-None-Match": etag } : {} });
  if (res.status === 304) return null;
  await check(res);
  return { roster: (await res.json()) as Roster, etag: res.headers.get("ETag") };
}

// fetchChat returns null when unchanged (304 via ETag).
export async function fetchChat(
  machineId: string,
  paneId: string, // "" with a dir: a transcript on its own, read-only
  sessionId: string | null,
  etag: string | null,
  dir?: string,
): Promise<{ body: ChatResponse; etag: string | null } | null> {
  const q = new URLSearchParams({ machine: machineId });
  if (paneId) q.set("pane", paneId);
  else if (dir) q.set("dir", dir);
  if (sessionId) q.set("session", sessionId);
  const res = await fetch(`/api/chat?${q}`, { headers: etag ? { "If-None-Match": etag } : {} });
  if (res.status === 304) return null;
  await check(res);
  return { body: (await res.json()) as ChatResponse, etag: res.headers.get("ETag") };
}

// ---- review: what changed in a folder since the viewer last looked ----
export type ReviewFile = { path: string; added: number; removed: number; untracked?: boolean; fresh: boolean };
export type ReviewCommit = { sha: string; subject: string; when: number };
export type Review = {
  repo?: string;
  branch?: string;
  head?: string;
  baseline?: { head: string; at: string };
  commits: ReviewCommit[];
  files: ReviewFile[];
  fresh: number;
  error?: string;
};
export const fetchReview = (machineId: string, dir: string): Promise<Review> =>
  fetch(withMachine(`/api/review?dir=${encodeURIComponent(dir)}`, machineId))
    .then(check)
    .then((r) => r.json() as Promise<Review>);
export const fetchReviewDiff = (machineId: string, dir: string, path: string): Promise<string> =>
  fetch(withMachine(`/api/review/diff?dir=${encodeURIComponent(dir)}&path=${encodeURIComponent(path)}`, machineId))
    .then(check)
    .then((r) => r.json() as Promise<{ diff: string }>)
    .then((d) => d.diff);
export const markReviewed = (machineId: string, dir: string): Promise<void> => sendJSON("POST", "/api/review/seen", { machine: machineId, dir });

// ---- search: full text over every transcript ----
export type SearchHit = { machine: string; project: string; dir: string; session: string; uuid: string; ts: string; role: "user" | "assistant"; snippet: string };
// encodePath is the name Claude Code gives a folder's project directory —
// the same rule as transcript.EncodePath on the server
export const encodePath = (dir: string) => dir.replace(/[^A-Za-z0-9]/g, "-");
export type SearchResult = { hits: SearchHit[]; errors?: Record<string, string>; more: boolean };
export const searchTranscripts = (q: string, f: { machine?: string; dir?: string; days?: number }, signal?: AbortSignal): Promise<SearchResult> => {
  const p = new URLSearchParams({ q });
  if (f.machine) p.set("machine", f.machine);
  if (f.dir) p.set("dir", f.dir);
  if (f.days) p.set("days", String(f.days));
  return fetch(`/api/search?${p}`, { signal })
    .then(check)
    .then((r) => r.json() as Promise<SearchResult>);
};

// ---- usage: what the agents cost over a period ----
export type UsageTotals = { input: number; output: number; cache_read: number; cache_write: number; cache_write_1h?: number; cost: number; turns: number; unknown: number };
export type UsageReport = {
  since: string;
  total: UsageTotals;
  days: (UsageTotals & { date: string })[];
  projects: (UsageTotals & { project: string; dir: string; sessions: number })[];
  models: (UsageTotals & { model: string; name: string })[];
  sessions: (UsageTotals & { session: string; project: string; dir: string; model: string; first: string; last: string })[];
  files: number;
};
export type UsageMachine = { id: string; label: string; report?: UsageReport; error?: string };
export const fetchUsage = (days: number, machine: string, tz: string): Promise<{ since: string; days: number; machines: UsageMachine[] }> => {
  const p = new URLSearchParams({ days: String(days), tz });
  if (machine) p.set("machine", machine);
  return fetch(`/api/usage?${p}`)
    .then(check)
    .then((r) => r.json() as Promise<{ since: string; days: number; machines: UsageMachine[] }>);
};

// ---- quick prompts: saved texts for the composer ----
export type QuickPrompt = { id?: string; label: string; text: string; dir?: string };
export const fetchQuick = (): Promise<QuickPrompt[]> =>
  fetch("/api/quick")
    .then(check)
    .then((r) => r.json() as Promise<QuickPrompt[]>);
export const saveQuick = (list: QuickPrompt[]): Promise<QuickPrompt[]> =>
  fetch("/api/quick", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(list) })
    .then(check)
    .then((r) => r.json() as Promise<QuickPrompt[]>);

// ---- schedules: prompts to an agent on a timetable ----
export type Schedule = {
  id?: string;
  machine: string;
  dir: string;
  label: string;
  cron: string;
  prompt: string;
  enabled: boolean;
  next?: string; // the coming run, filled in by the list
  last_run?: string;
  last_result?: string;
  last_ok?: boolean;
};
export const fetchSchedules = (): Promise<Schedule[]> =>
  fetch("/api/schedules")
    .then(check)
    .then((r) => r.json() as Promise<Schedule[]>);
export const putSchedule = (s: Schedule): Promise<Schedule> =>
  fetch("/api/schedules", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(s) })
    .then(check)
    .then((r) => r.json() as Promise<Schedule>);
export const deleteSchedule = (id: string): Promise<void> => sendJSON("DELETE", `/api/schedules/${encodeURIComponent(id)}`);
export const runSchedule = (id: string): Promise<Schedule> => sendJSON<Schedule>("POST", `/api/schedules/${encodeURIComponent(id)}/run`);

export type CodeStatus = { available: boolean; port: number; error?: string };

// whether the machine's code-server answers (force re-probes a cached "no")
export const codeStatus = (machineId: string, force = false): Promise<CodeStatus> =>
  fetch(`/api/code/${encodeURIComponent(machineId)}${force ? "?check=1" : ""}`)
    .then(check)
    .then((r) => r.json() as Promise<CodeStatus>);

// the pane is on screen: a held "done" in the sidebar comes down
export function markSeen(machine: string, pane: string): Promise<void> {
  return sendJSON("POST", "/api/seen", { machine, pane });
}

export function submitPrompt(machine: string, pane: string, text: string): Promise<void> {
  return sendJSON("POST", "/api/submit", { machine, pane, text });
}

export function sendKeys(machine: string, pane: string, keys: string[]): Promise<void> {
  return sendJSON("POST", "/api/keys", { machine, pane, keys });
}

// Created is what the create calls return: the container exists either way;
// a warning says the agent in it failed to start (so the caller lands on the
// pane and shows the warning instead of leaving a dialog that would create a
// second one).
export type Created = { pane: string; warning?: string };

async function create(url: string, req: unknown): Promise<Created> {
  const body = await sendJSON<{ pane: string; agent_error?: string }>("POST", url, req);
  return {
    pane: body.pane,
    warning: body.agent_error ? `Created, but the agent failed to start: ${body.agent_error}` : undefined,
  };
}

export const createSession = (req: { machine: string; name: string; dir: string; agent: string }) =>
  create("/api/sessions", req);

export const createTab = (req: { machine: string; workspace: string; name: string; agent: string }) =>
  create("/api/tabs", req);

export const createWorktree = (req: {
  machine: string;
  workspace: string;
  branch: string;
  base: string;
  name: string;
  agent: string;
}) => create("/api/worktrees", req);

export const killSession = (machineId: string, id: string): Promise<void> =>
  sendJSON("DELETE", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId));

export const closeTab = (machineId: string, id: string): Promise<void> =>
  sendJSON("DELETE", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId));

export const renameTab = (machineId: string, id: string, label: string): Promise<void> =>
  sendJSON("PATCH", withMachine(`/api/tabs/${encodeURIComponent(id)}`, machineId), { label });

export const renameSession = (machineId: string, id: string, label: string): Promise<void> =>
  sendJSON("PATCH", withMachine(`/api/sessions/${encodeURIComponent(id)}`, machineId), { label });

export const addMachine = (req: { label: string; host: string }): Promise<void> =>
  sendJSON("POST", "/api/machines", req);

export const removeMachine = (id: string): Promise<void> =>
  sendJSON("DELETE", `/api/machines/${encodeURIComponent(id)}`);

// ---- files ----

export type FileInfo = {
  path: string;
  kind: "text" | "image" | "media" | "pdf" | "binary" | "dir" | "missing";
  size: number;
  candidates?: string[];
};

export type FileDiff = { repo?: string; untracked?: boolean; diff: string };

export async function fsStat(machineId: string, paneId: string, path: string): Promise<FileInfo> {
  const q = new URLSearchParams({ machine: machineId, pane: paneId, path });
  const res = await check(await fetch(`/api/fs/stat?${q}`));
  return (await res.json()) as FileInfo;
}

export function fileUrl(machineId: string, path: string): string {
  return `/api/fs/file?${new URLSearchParams({ machine: machineId, path })}`;
}

export async function fsText(
  machineId: string,
  path: string,
): Promise<{ text: string; truncated: boolean; max: number }> {
  const res = await check(await fetch(fileUrl(machineId, path)));
  return {
    text: await res.text(),
    truncated: res.headers.get("X-Herdr-UI-Truncated") === "true",
    max: Number(res.headers.get("X-Herdr-UI-Max") ?? 0),
  };
}

export type Upload = { name: string; path: string; rel: string; size: number; kind: string };

// uploadFiles stores files in the pane's folder (.herdr-ui/uploads) and
// returns their paths; up to 4 per call, 25 MB each.
export async function uploadFiles(machineId: string, paneId: string, files: File[]): Promise<Upload[]> {
  const form = new FormData();
  for (const f of files) form.append("file", f, f.name);
  const q = new URLSearchParams({ machine: machineId, pane: paneId });
  const res = await check(await fetch(`/api/fs/upload?${q}`, { method: "POST", body: form }));
  return ((await res.json()) as { files: Upload[] }).files;
}

export async function fsDiff(machineId: string, path: string): Promise<FileDiff> {
  const q = new URLSearchParams({ machine: machineId, path });
  const res = await check(await fetch(`/api/fs/diff?${q}`));
  return (await res.json()) as FileDiff;
}
