// The files an agent touched in a conversation, read from its own tool calls: what it edited, what it only
// read, the newest touch first. A tool's input cut to a string at 8 KB on the server still names its file.

import type { Part, Turn } from "./api";

export const EDIT_TOOLS = new Set(["Edit", "Write", "MultiEdit", "NotebookEdit"]);
export const READ_TOOLS = new Set(["Read"]);

export type Touch = {
  path: string;
  edits: number;
  reads: number;
  at: number; // unix ms of the last touch, 0 when the turn carries no time
  seq: number; // the last touch's place in the conversation
  failed: boolean; // the last call on it errored
};

export type Footprint = { edited: Touch[]; read: Touch[] };

const PATH_KEYS = ["file_path", "notebook_path", "path"] as const;
const cutPathRe = /"(?:file_path|notebook_path)":"((?:[^"\\]|\\.)*)"/;

// toolPath is the file a tool call names: file_path, notebook_path or path in its input, found even when the input was cut to a string
export function toolPath(part: Part): string | null {
  const input = part.input;
  if (input && typeof input === "object") {
    for (const key of PATH_KEYS) {
      const value = (input as Record<string, unknown>)[key];
      if (typeof value === "string" && value) return value;
    }
    return null;
  }
  if (typeof input !== "string") return null;
  const match = cutPathRe.exec(input);
  if (!match) return null;
  try {
    return JSON.parse(`"${match[1]}"`) as string;
  } catch {
    return null;
  }
}

// footprint groups the edits and reads by file, each file with its last touch; edited files first by that touch, then the ones only read
export function footprint(turns: Turn[]): Footprint {
  const touches = new Map<string, Touch>();
  let seq = 0;
  for (const turn of turns) {
    if (turn.role !== "assistant") continue;
    const at = turn.ts ? Date.parse(turn.ts) || 0 : 0;
    for (const part of turn.parts) {
      if (part.type !== "tool") continue;
      const name = part.tool_name ?? "";
      const edit = EDIT_TOOLS.has(name);
      if (!edit && !READ_TOOLS.has(name)) continue;
      const path = toolPath(part);
      if (!path) continue;
      seq++;
      const touch = touches.get(path) ?? { path, edits: 0, reads: 0, at: 0, seq: 0, failed: false };
      if (edit) touch.edits++;
      else touch.reads++;
      touch.at = at;
      touch.seq = seq;
      touch.failed = !!part.is_error;
      touches.set(path, touch);
    }
  }
  const newestFirst = (a: Touch, b: Touch) => b.seq - a.seq;
  const all = [...touches.values()];
  return { edited: all.filter((touch) => touch.edits > 0).sort(newestFirst), read: all.filter((touch) => touch.edits === 0).sort(newestFirst) };
}

// shownPath is the path as the rail shows it: relative to the pane's folder when under it
export function shownPath(path: string, dir: string): string {
  const base = dir.replace(/\/+$/, "");
  return base && path.startsWith(base + "/") ? path.slice(base.length + 1) : path;
}

// toolLabel is what a tool call says in one line: its name, and the file or command it was about
export function toolLabel(part: Part): { name: string; hint: string; file: string | null } {
  const name = part.tool_name ?? "tool";
  const file = toolPath(part);
  // a file is a link when the tool read or wrote it; Grep's or Glob's path is a folder, named only
  if (file) return { name, hint: file.length > 70 ? "…" + file.slice(-69) : file, file: EDIT_TOOLS.has(name) || READ_TOOLS.has(name) ? file : null };
  const input = part.input as Record<string, unknown> | undefined | string;
  if (input && typeof input === "object") {
    const obj = input as Record<string, unknown>;
    const hint = obj["command"] ?? obj["pattern"] ?? obj["url"] ?? obj["description"] ?? obj["query"];
    if (typeof hint === "string") {
      return { name, hint: hint.length > 60 ? hint.slice(0, 60) + "…" : hint, file: null };
    }
  }
  return { name, hint: "", file: null };
}
