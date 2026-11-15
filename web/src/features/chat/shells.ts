// The shells an agent started in the background: Claude Code's Bash calls with run_in_background, which its terminal
// lists in the footer. The transcript says when one starts (the call's result names its id and the file its output goes
// to), when the agent stops it, and how it ended (a task notification naming the call). Pure; read from the turns shown,
// so a shell started before them is not known.

import { tn } from "@/lib/i18n";
import type { Part, Turn } from "./api";
import { noteKind, tagged } from "./notes";

export type ShellStatus = "running" | "completed" | "failed" | "killed" | "stopped";

export type Shell = {
  toolId: string; // the Bash call that started it
  id: string; // Claude Code's id for the task
  command: string;
  description: string; // the call's own, "" when it gave none
  output: string; // the file its output is written to, "" when the result named none
  started?: string;
  ended?: string;
  status: ShellStatus;
  summary: string; // how it ended, in Claude Code's words
};

const STARTED = /^Command running in background with ID: ([\w-]+)/;
const OUTPUT_FILE = /Output is being written to: (\S+?\.output)(?=[.\s]|$)/;
const STOP_TOOLS = new Set(["TaskStop", "KillShell", "KillBash"]);
const STOP_KEYS = ["task_id", "shell_id", "bash_id"] as const;
const ENDINGS = new Set<ShellStatus>(["completed", "failed", "killed"]);
export const LABEL_MAX = 60;

// inputWord reads a string field of a call's input: an object, or the string the server cut a big one to
function inputWord(input: unknown, key: string): string {
  if (input && typeof input === "object") {
    const value = (input as Record<string, unknown>)[key];
    return typeof value === "string" ? value : "";
  }
  if (typeof input !== "string") return "";
  const found = new RegExp(`"${key}":"((?:[^"\\\\]|\\\\.)*)"`).exec(input);
  if (!found) return "";
  try {
    return JSON.parse(`"${found[1]}"`) as string;
  } catch {
    return found[1] ?? "";
  }
}

// startedBy reads a Bash call as the start of a background shell, or null for any other call
function startedBy(part: Part): Shell | null {
  if (part.type !== "tool" || part.tool_name !== "Bash" || !part.tool_id || part.is_error) return null;
  const id = STARTED.exec(part.output ?? "")?.[1];
  if (!id) return null;
  return {
    toolId: part.tool_id,
    id,
    command: inputWord(part.input, "command"),
    description: inputWord(part.input, "description"),
    output: OUTPUT_FILE.exec(part.output ?? "")?.[1] ?? "",
    started: part.at,
    status: "running",
    summary: "",
  };
}

// shellsOf is every background shell the turns show starting, in the order started, each as it stands at their end
export function shellsOf(turns: Turn[]): Shell[] {
  const shells: Shell[] = [];
  const find = (toolId: string, id: string) => shells.find((shell) => (toolId !== "" && shell.toolId === toolId) || (id !== "" && shell.id === id));
  for (const turn of turns) {
    for (const part of turn.parts) {
      const shell = startedBy(part);
      if (shell) {
        shells.push(shell);
        continue;
      }
      if (part.type === "tool" && STOP_TOOLS.has(part.tool_name ?? "") && part.output !== undefined && !part.is_error) {
        const stopped = find("", STOP_KEYS.map((key) => inputWord(part.input, key)).find((id) => id !== "") ?? "");
        if (stopped?.status === "running") Object.assign(stopped, { status: "stopped", ended: part.at ?? turn.ts });
        continue;
      }
      if (turn.role !== "user" || part.type !== "text" || noteKind(part.text ?? "") !== "task") continue;
      const text = part.text ?? "";
      const ended = find(tagged(text, "tool-use-id"), tagged(text, "task-id"));
      if (!ended) continue;
      const status = tagged(text, "status") as ShellStatus;
      // a shell the agent stopped is told killed a moment later: it stays stopped, with what Claude Code said of it
      if (ended.status === "running") Object.assign(ended, { status: ENDINGS.has(status) ? status : "completed", ended: turn.ts });
      ended.summary = tagged(text, "summary");
    }
  }
  return shells;
}

// runningShells are the ones still at work
export const runningShells = (shells: Shell[]): Shell[] => shells.filter((shell) => shell.status === "running");

// shellOfCall is the shell a call started, for the call's own line
export const shellOfCall = (shells: Shell[] | undefined, toolId: string | undefined): Shell | undefined => (toolId ? shells?.find((shell) => shell.toolId === toolId) : undefined);

// shellLabel is a shell in a few words: what the agent said it is for, else its command's first line
export function shellLabel(shell: Shell): string {
  const words = (shell.description || shell.command.split("\n")[0] || shell.id).trim();
  return words.length > LABEL_MAX ? `${words.slice(0, LABEL_MAX - 1).trimEnd()}…` : words;
}

// shellSeconds is how long a shell ran, or has run so far at now; null without a start
export function shellSeconds(shell: Shell, now: number): number | null {
  const from = shell.started ? Date.parse(shell.started) : NaN;
  const to = shell.ended ? Date.parse(shell.ended) : now;
  return Number.isFinite(from) && Number.isFinite(to) ? Math.max(0, (to - from) / 1000) : null;
}

// runningWords is the head of the line over the composer
export const runningWords = (count: number): string => tn(count, "{n} shell running in the background", "{n} shells running in the background");

// sameShells says two readings draw the same, so a poll that moved no shell redraws no call
export function sameShells(a: Shell | undefined, b: Shell | undefined): boolean {
  return a === b || (!!a && !!b && a.id === b.id && a.status === b.status && a.ended === b.ended && a.summary === b.summary);
}
