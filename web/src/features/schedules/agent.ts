// The pure part of an agent schedule — a fresh agent started in a folder with a prompt on a timetable: the scout's
// prompt the form suggests, a fresh record, and the words the list says about one.

import type { Schedule } from "./api";
import { t } from "@/lib/i18n";

// SCOUT_PROMPT is what the form suggests: look around the project and make tasks for what could be better
export const SCOUT_PROMPT =
  "Look through this project for what could be better — bugs, missing tests, rough edges, outdated docs — and create a task for each in the project “<name>” with a clear title and a description that says where and why. Skip anything a task already covers.";

// agentSchedule is what an agent schedule looks like as the form hands it over: the tab closed once answered unless told otherwise
export const agentSchedule = (s: Partial<Schedule> & { machine: string; dir: string; agent: string; prompt: string }): Schedule => ({
  id: s.id,
  kind: "agent",
  machine: s.machine,
  dir: s.dir,
  label: s.label ?? "",
  cron: s.cron ?? "",
  prompt: s.prompt,
  agent: s.agent,
  model: s.model ?? "",
  args: s.args ?? "",
  close: s.close ?? true,
  enabled: s.enabled ?? true,
});

// agentWords is an agent schedule's row: the machine, the folder, the agent, and whether its tab goes once it has answered
export function agentWords(s: Pick<Schedule, "close">, machine: string, folder: string, agent: string): string {
  return `${machine} · ${folder} · ${agent}${s.close ? t(" · closes when answered") : ""}`;
}

// watchedWords says which pane the schedule still watches for its answer, "" when none
export const watchedWords = (s: Pick<Schedule, "last_pane">): string => (s.last_pane ? t("watching {pane}", { pane: s.last_pane.pane }) : "");
