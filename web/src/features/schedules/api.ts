// Schedules: prompts to an agent on a timetable, the standup pushed as a digest, or a fresh agent started in a folder.

import { getJSON, sendJSON } from "@/lib/http";

export type ScheduleKind = "prompt" | "digest" | "agent";
export type DigestSince = "midnight" | "24h" | "7d" | "last";

// Started is where an agent schedule's last run put its agent, while the schedule watches it for its answer
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
  enabled: boolean;
  next?: string; // the coming run, filled in by the list
  last_run?: string;
  last_result?: string;
  last_ok?: boolean;
  last_pane?: Started;
};
export const fetchSchedules = (): Promise<Schedule[]> => getJSON("/api/schedules");
export const putSchedule = (s: Schedule): Promise<Schedule> => sendJSON("PUT", "/api/schedules", s);
export const deleteSchedule = (id: string): Promise<void> => sendJSON("DELETE", `/api/schedules/${encodeURIComponent(id)}`);
export const runSchedule = (id: string): Promise<Schedule> => sendJSON<Schedule>("POST", `/api/schedules/${encodeURIComponent(id)}/run`);
