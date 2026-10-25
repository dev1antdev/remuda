// The Code mode: whether a machine's code-server answers.

import { getJSON, sendJSON } from "@/lib/http";

export type CodeStatus = { available: boolean; port: number; authority?: string; version?: string; error?: string }; // authority: what the workbench names its files by

// whether the machine's code-server answers (force re-probes a cached "no")
export const codeStatus = (machineId: string, force = false): Promise<CodeStatus> => getJSON(`/api/code/${encodeURIComponent(machineId)}${force ? "?check=1" : ""}`);

// openInEditor opens a file in the editor already on screen on that machine, at a line when given; no frame reloads
export const openInEditor = (machineId: string, path: string, line?: number): Promise<{ ok: boolean }> => sendJSON("POST", `/api/code/${encodeURIComponent(machineId)}/open`, { path, line: line ?? 0 });
