// A workspace put to sleep and woken, and the two switches beside them: wake one when it is opened, resume the chats a
// reboot stopped by themselves.

import { check, getJSON, sendJSON } from "@/lib/http";

// Busy is an agent still at work or asking when a sleep was asked for
export type Busy = { pane: string; agent: string; status: "working" | "blocked" };

// Started is a sleep's or a wake's job and how many agents it is about; busy instead when the server wants a yes first
export type Started = { job: string; agents: number; busy?: undefined } | { busy: Busy[]; error: string; job?: undefined };

export type SleepSettings = { wake_on_open: boolean; resume_automatically: boolean };

const path = (workspace: string, what: "sleep" | "wake") => `/api/sessions/${encodeURIComponent(workspace)}/${what}`;

// sleepWorkspace asks the workspace's agents to leave; agents the server sees at work or asking come back as busy
// unless anyway says the person agreed
export async function sleepWorkspace(machine: string, workspace: string, anyway: boolean): Promise<Started> {
  const res = await fetch(path(workspace, "sleep"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ machine, anyway }) });
  if (res.status === 409) {
    const body = (await res
      .clone()
      .json()
      .catch(() => null)) as { busy?: Busy[]; error?: string } | null;
    if (body?.busy?.length) return { busy: body.busy, error: body.error ?? "" };
  }
  await check(res);
  return (await res.json()) as Started;
}

export const wakeWorkspace = (machine: string, workspace: string): Promise<{ job: string; agents: number }> => sendJSON("POST", path(workspace, "wake"), { machine });

export const fetchSleepSettings = (): Promise<SleepSettings> => getJSON("/api/sleep/settings");

export const saveSleepSettings = (settings: SleepSettings): Promise<SleepSettings> => sendJSON("PUT", "/api/sleep/settings", settings);
