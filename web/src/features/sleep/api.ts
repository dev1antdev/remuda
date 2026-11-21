// A workspace put to sleep and woken, and the two switches beside them: wake one when it is opened, resume the chats a
// reboot stopped by themselves.

import { getJSON, postJSONOr409, sendJSON } from "@/lib/http";

// Busy is an agent still at work or asking when a sleep was asked for
export type Busy = { pane: string; agent: string; status: "working" | "blocked" };

// Started is a sleep's or a wake's job and how many agents it is about; busy instead when the server wants a yes first
export type Started = { job: string; agents: number; busy?: undefined } | { busy: Busy[]; error: string; job?: undefined };

export type SleepSettings = { wake_on_open: boolean; resume_automatically: boolean };

const path = (workspace: string, what: "sleep" | "wake") => `/api/sessions/${encodeURIComponent(workspace)}/${what}`;

// sleepWorkspace asks the workspace's agents to leave; agents the server sees at work or asking come back as busy
// unless anyway says the person agreed
export const sleepWorkspace = (machine: string, workspace: string, anyway: boolean): Promise<Started> =>
  postJSONOr409<Started>(path(workspace, "sleep"), { machine, anyway }, (conflict) => !!conflict.busy?.length);

export const wakeWorkspace = (machine: string, workspace: string): Promise<{ job: string; agents: number }> => sendJSON("POST", path(workspace, "wake"), { machine });

export const fetchSleepSettings = (): Promise<SleepSettings> => getJSON("/api/sleep/settings");

export const saveSleepSettings = (settings: SleepSettings): Promise<SleepSettings> => sendJSON("PUT", "/api/sleep/settings", settings);
