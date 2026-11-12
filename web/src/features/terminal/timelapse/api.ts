// The timelapses' server calls: a pane's and its folder's recordings, one recording whole, the search over their
// screens, the deletions, and the switch with what the folder holds, for Settings.

import { getJSON, sendJSON } from "@/lib/http";

export type RecordingMeta = {
  id: string;
  machine: string;
  pane: string;
  workspace: string;
  dir: string;
  folder: string; // the sidebar's folder, which the screen-sharing mode hides by
  agent: string; // herdr's kind, "claude"
  title: string;
  cols: number;
  rows: number;
  start: string;
  end: string;
  count: number; // frames
  ended: string; // done, idle, blocked, gone, stopped, cut, away
  size: number;
};
// a frame is the screen `at` ms after the start: `lines` rows, of which `rows` are the ones that moved (every one in the first)
export type RecordingFrame = { at: number; lines: number; rows?: { line: number; text: string }[] };
export type Recording = RecordingMeta & { frames: RecordingFrame[] };
export type TimelapseStats = { on: boolean; scratch: boolean; count: number; bytes: number; max: number }; // scratch: a second remuda beside the real one, which records nothing

export const listRecordings = (machine: string, pane: string, dir: string): Promise<{ recordings: RecordingMeta[]; on: boolean }> =>
  getJSON(`/api/timelapse/recordings?${new URLSearchParams({ machine, pane, dir })}`);
export const fetchRecording = (id: string): Promise<Recording> => getJSON(`/api/timelapse/recordings/${encodeURIComponent(id)}`);
export const deleteRecording = (id: string): Promise<{ ok: boolean }> => sendJSON("DELETE", `/api/timelapse/recordings/${encodeURIComponent(id)}`);

// an appearance is a stretch the text stood on the screen: from frame `frame`, `at` ms after the recording's start, for `for` ms, in row `line`
export type Appearance = { frame: number; at: number; for: number; line: number };
// a screen hit is one recording that showed the text: where it first stood with the screen's rows around it, cut at column `col`, and when it came back
export type ScreenHit = Pick<RecordingMeta, "machine" | "pane" | "workspace" | "dir" | "folder" | "agent" | "title" | "start"> & {
  recording: string;
  first: Appearance;
  again?: Appearance[];
  times: number; // every appearance, the ones past those listed too
  before: string[];
  row: string;
  after: string[];
  col: number;
};
// cut says why recordings of the scope went unread: the query's budget of bytes read, or its time; more: older recordings may show it too
export type ScreenResult = { hits: ScreenHit[]; searched: number; unread: number; cut?: "budget" | "time"; more: boolean };
export type ScreenScope = { machine?: string; pane?: string; dir?: string; days?: number };

export const searchScreens = (q: string, scope: ScreenScope, signal?: AbortSignal): Promise<ScreenResult> => {
  const params = new URLSearchParams({ q });
  for (const [name, value] of Object.entries(scope)) if (value) params.set(name, String(value));
  return getJSON(`/api/timelapse/search?${params}`, { signal });
};

export const fetchTimelapse = (): Promise<TimelapseStats> => getJSON("/api/timelapse");
export const saveTimelapse = (on: boolean): Promise<TimelapseStats> => sendJSON("PUT", "/api/timelapse", { on });
export const deleteRecordings = (): Promise<{ deleted: number; stats: TimelapseStats }> => sendJSON("DELETE", "/api/timelapse");
