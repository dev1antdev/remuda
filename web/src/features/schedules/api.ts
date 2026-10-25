// Schedules: prompts to an agent on a timetable, or the standup pushed as a digest.

import { getJSON, sendJSON } from "@/lib/http";

export type ScheduleKind = "prompt" | "digest";
export type DigestSince = "midnight" | "24h" | "7d" | "last";

export type Schedule = {
  id?: string;
  kind?: ScheduleKind; // prompt when absent (a record from before the kinds)
  machine: string;
  dir: string;
  label: string;
  cron: string;
  prompt: string;
  since?: DigestSince; // a digest's period
  enabled: boolean;
  next?: string; // the coming run, filled in by the list
  last_run?: string;
  last_result?: string;
  last_ok?: boolean;
};
export const fetchSchedules = (): Promise<Schedule[]> => getJSON("/api/schedules");
export const putSchedule = (s: Schedule): Promise<Schedule> => sendJSON("PUT", "/api/schedules", s);
export const deleteSchedule = (id: string): Promise<void> => sendJSON("DELETE", `/api/schedules/${encodeURIComponent(id)}`);
export const runSchedule = (id: string): Promise<Schedule> => sendJSON<Schedule>("POST", `/api/schedules/${encodeURIComponent(id)}/run`);
