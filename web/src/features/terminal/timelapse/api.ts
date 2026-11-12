// The timelapses' server calls: a pane's and its folder's recordings, one recording whole, the deletions, and the
// switch with what the folder holds, for Settings.

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

export const fetchTimelapse = (): Promise<TimelapseStats> => getJSON("/api/timelapse");
export const saveTimelapse = (on: boolean): Promise<TimelapseStats> => sendJSON("PUT", "/api/timelapse", { on });
export const deleteRecordings = (): Promise<{ deleted: number; stats: TimelapseStats }> => sendJSON("DELETE", "/api/timelapse");
