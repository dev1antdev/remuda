// The server's side of undoing an agent's turn: the folder's turns, what an undo would do now, the undo itself.

import { getJSON, sendJSON } from "@/lib/http";

// Was is what the turn did to a file: the undo removes an added one, brings back a deleted one, puts back the rest
export type Was = "added" | "deleted" | "changed" | "mode" | "committed";

export type UndoFile = {
  path: string;
  was: Was;
  committed?: boolean; // the turn's commits changed it
  binary?: boolean;
  dirty_at: boolean; // uncommitted when the turn began
  dirty_to: boolean; // uncommitted when it ended
};

export type UndoCommit = { sha: string; subject: string; when: number; merge?: boolean };

export type UndoTurn = {
  id: string;
  machine: string;
  dir: string;
  root: string;
  pane: string;
  agent?: string;
  started: string;
  ended: string;
  branch?: string;
  files: UndoFile[] | null;
  commits?: UndoCommit[];
  size: number;
  why?: string; // why it can never be undone
  shared?: boolean; // another agent worked in the folder at the same time
  undone?: { at: string; files: number; reverted?: string[] };
};

// ListedTurn is a turn as a folder's list carries it: its files counted, the plan lists them
export type ListedTurn = Omit<UndoTurn, "files"> & { files: number };

export type UndoPlan = {
  turn: UndoTurn;
  refused?: { why: string; files?: string[] };
  revert_why?: string; // why the commits cannot be reverted now
  pushed?: string[]; // the remote-tracking branches holding the turn's commits
  left_as_is?: string[]; // without the reverts, the files only the commits can put back
  uncommitted: number;
  head_moved?: boolean;
};

export type UndoResult = { files: number; reverted?: string[]; pushed?: string[] };

export const fetchTurns = (machineId: string, dir: string) =>
  getJSON<{ turns: ListedTurn[]; recording: boolean }>(`/api/undo?machine=${encodeURIComponent(machineId)}&dir=${encodeURIComponent(dir)}`);

export const fetchPlan = (machineId: string, id: string) => getJSON<UndoPlan>(`/api/undo/${encodeURIComponent(id)}/plan?machine=${encodeURIComponent(machineId)}`);

export const undoTurn = (machineId: string, id: string, revert: boolean) => sendJSON<UndoResult>("POST", `/api/undo/${encodeURIComponent(id)}`, { machine: machineId, revert });
