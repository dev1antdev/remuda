// The pure part of an agent schedule — a fresh agent started in a folder with a prompt on a timetable: the scout's
// prompt the form suggests, a fresh record, and the words the list says about one.

import { canDenyPrompts, deniedCount, deniedList } from "@/features/agents/deny";
import type { Schedule } from "./api";
import { t } from "@/lib/i18n";

// SCOUT_PROMPT is what the form suggests: look around the project and make tasks for what could be better
export const SCOUT_PROMPT =
  "Look through this project for what could be better — bugs, missing tests, rough edges, outdated docs — and create a task for each in the project “<name>” with a clear title and a description that says where and why. Skip anything a task already covers.";

// agentSchedule is what an agent schedule looks like as the form hands it over: the tab closed once answered and, for
// Claude Code, prompts denied, unless told otherwise
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
  deny_prompts: s.deny_prompts ?? true,
  enabled: s.enabled ?? true,
});

// agentWords is an agent schedule's row: the machine, the folder, the agent, whether its tab goes once it has answered
// and whether it denies what would wait on a permission prompt
export function agentWords(s: Pick<Schedule, "close" | "deny_prompts" | "agent">, machine: string, folder: string, agent: string): string {
  const denies = s.deny_prompts === true && canDenyPrompts(s.agent ?? "");
  return `${machine} · ${folder} · ${agent}${s.close ? t(" · closes when answered") : ""}${denies ? t(" · denies instead of asking") : ""}`;
}

// deniedWords is what the row says of the calls the last run's agent was denied, "" for none
export const deniedWords = (s: Pick<Schedule, "last_denied">): string =>
  s.last_denied?.length ? `${deniedCount(s.last_denied.length)}: ${deniedList(s.last_denied)}` : "";

// watchedWords says which pane the schedule still watches for its answer, "" when none
export const watchedWords = (s: Pick<Schedule, "last_pane">): string => (s.last_pane ? t("watching {pane}", { pane: s.last_pane.pane }) : "");
