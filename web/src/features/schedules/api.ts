// Schedules: prompts to an agent on a timetable, the standup pushed as a digest, or a fresh agent started in a folder.

import type { Denial } from "@/features/agents/deny";
import { getJSON, sendJSON } from "@/lib/http";

export type ScheduleKind = "prompt" | "digest" | "agent";
export type DigestSince = "midnight" | "24h" | "7d" | "last";
// why a run was skipped: the agent still works, waits for an answer, has not taken its prompt yet, its machine has not said,
// the run before is still on its way, or its agent's plan window holds a new start (an offer to run it on the other vendor goes out)
export type SkipReason = "working" | "asking" | "starting" | "unsure" | "running" | "limit";

// Started is where an agent schedule's last run put its agent, watched until it has answered: the next run waits for it
export type Started = { machine: string; pane: string; tab: string; workspace: string; made_workspace: boolean; worked: boolean; at: string };

export type Schedule = {
  id?: string;
  kind?: ScheduleKind; // prompt when absent (a record from before the kinds)
  machine: string;
  dir: string;
  label: string;
  cron: string;
  prompt: string;
  since?: DigestSince; // a digest's period
  agent?: string; // an agent schedule's kind of agent
  model?: string; // the model as the agent's own CLI names it, "" for its default
  args?: string; // extra arguments, as typed
  close?: boolean; // take the tab down once the agent has answered
  deny_prompts?: boolean; // its Claude Code denies what would wait on a permission prompt; absent on a record from before the switch (off)
  enabled: boolean;
  next?: string; // the coming run, filled in by the list
  last_run?: string;
  last_result?: string;
  last_ok?: boolean;
  skipped?: SkipReason; // set when the last run was skipped
  last_pane?: Started;
  last_denied?: Denial[]; // the calls the last run's agent was denied without asking
};
export const fetchSchedules = (): Promise<Schedule[]> => getJSON("/api/schedules");
export const putSchedule = (s: Schedule): Promise<Schedule> => sendJSON("PUT", "/api/schedules", s);
export const deleteSchedule = (id: string): Promise<void> => sendJSON("DELETE", `/api/schedules/${encodeURIComponent(id)}`);
// fetchNext is a cron's next runs on the host's clock, for the form before it is saved; a cron the server cannot read throws its reason
export const fetchNext = (cron: string): Promise<{ next: string[] }> => getJSON(`/api/schedules/next?cron=${encodeURIComponent(cron)}`);
export const runSchedule = (id: string): Promise<Schedule> => sendJSON<Schedule>("POST", `/api/schedules/${encodeURIComponent(id)}/run`);
