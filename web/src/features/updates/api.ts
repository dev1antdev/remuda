// Updates on a machine: what it can run, a run started, the jobs that ran.

import { getJSON, sendJSON } from "@/lib/http";

// an update the machine can run: the command the person sees before saying yes
export type Update = { what: string; title: string; command: string; note?: string };
export type Job = { id: string; machine: string; title: string; command: string; started: string; ended?: string; done: boolean; ok: boolean; error?: string; output: string };

export const listUpdates = (machineId: string): Promise<Update[]> =>
  getJSON<{ updates: Update[] }>(`/api/machines/${encodeURIComponent(machineId)}/updates`).then((r) => r.updates);
// the command shown goes back with the request: the server runs that or refuses
export const startUpdate = (machineId: string, what: string, command: string): Promise<{ job: string }> =>
  sendJSON("POST", `/api/machines/${encodeURIComponent(machineId)}/update`, { what, command });
export const listJobs = (machineId: string): Promise<{ jobs: Job[]; pending: number }> => getJSON(`/api/machines/${encodeURIComponent(machineId)}/jobs`);
export const fetchJob = (id: string): Promise<Job> => getJSON(`/api/jobs/${encodeURIComponent(id)}`);
// one chat started again on its session, by hand: a job, or the refusal (a shell, no session, a question open, still working)
export const restartPane = (machineId: string, paneId: string): Promise<{ job: string }> => sendJSON("POST", "/api/panes/restart", { machine: machineId, pane: paneId });
// the chats a reboot stopped on the machine in the panes picked (every one for none) started again on their sessions: a job, or the refusal when none is stopped
export const recoverChats = (machineId: string, panes?: string[]): Promise<{ job: string }> => sendJSON("POST", "/api/panes/recover", { machine: machineId, panes });

// Update everywhere: one step per pending update of a connected machine, the page's own host last
export type Step = { machine: string; label: string; what: string; title: string; command: string };
export type StepRun = Step & { state: "waiting" | "running" | "ok" | "failed" | "skipped"; job?: string; error?: string };
export type Batch = { id: string; started: string; ended?: string; done: boolean; steps: StepRun[] };
export const everywherePlan = (): Promise<Step[]> => getJSON<{ steps: Step[] }>("/api/updates/everywhere").then((r) => r.steps);
// the steps shown go back as they were: one that moved since is refused, and nothing runs
export const startEverywhere = (steps: Step[]): Promise<Batch> => sendJSON("POST", "/api/updates/everywhere", { steps });
export const fetchEverywhere = (id: string): Promise<Batch> => getJSON(`/api/updates/everywhere/${encodeURIComponent(id)}`);

// Fleet setup: every machine side by side, a row per fact, the norm most machines share and the odd one out marked
export type FleetAction = { what: string; title: string; command: string };
export type FleetCell = { value: string; mark?: "odd" | "split"; at?: number; note?: string; action?: FleetAction };
export type FleetRow = {
  id: string;
  arg?: string;
  kind: "version" | "via" | "flag" | "hook" | "plugin" | "model" | "hash" | "text" | "behind";
  state: "same" | "odd" | "split" | "single";
  norm?: string;
  cells: Record<string, FleetCell>; // by machine id; a machine without one is a machine the row does not apply to
  hand?: string[]; // the machines off the row's best that no update brings in line: theirs to do by hand
};
export type FleetSection = { id: "agents" | "herdr" | "remuda" | "claude" | "instructions" | "repo"; name?: string; folders?: Record<string, string>; rows: FleetRow[] };
export type FleetColumn = { id: string; label: string; away?: boolean; error?: string };
export type FleetTable = { at: string; machines: FleetColumn[]; sections: FleetSection[]; single?: { name: string; machine: string; folder: string }[] };
// fresh reads every machine at once, its vitals too; otherwise a read stands a minute
export const fleetSetup = (fresh = false): Promise<FleetTable> => getJSON(`/api/fleet-setup${fresh ? "?fresh=1" : ""}`);
