// The Code mode: whether a machine's code-server answers.

import { getJSON, sendJSON } from "@/lib/http";
import type { Flavor } from "@/lib/theme";

export type CodeStatus = { available: boolean; port: number; authority?: string; version?: string; error?: string }; // authority: what the workbench names its files by

// whether the machine's code-server answers (force re-probes a cached "no")
export const codeStatus = (machineId: string, force = false): Promise<CodeStatus> => getJSON(`/api/code/${encodeURIComponent(machineId)}${force ? "?check=1" : ""}`);

// openInEditor opens a file in the editor already on screen on that machine, at a line when given; no frame reloads
export const openInEditor = (machineId: string, path: string, line?: number): Promise<{ ok: boolean }> => sendJSON("POST", `/api/code/${encodeURIComponent(machineId)}/open`, { path, line: line ?? 0 });

// codeWorkspace writes the machine's workspaces into its one .code-workspace file for the editor and names it
export const codeWorkspace = (machineId: string): Promise<{ path: string; folders: string[] }> => sendJSON("POST", `/api/code/${encodeURIComponent(machineId)}/workspace`, {});

// codeTheme dresses the machine's editor in the page's Catppuccin flavour; the extension is installed on the way when missing
export const codeTheme = (machineId: string, theme: Flavor): Promise<{ theme: string; installed: boolean }> => sendJSON("POST", `/api/code/${encodeURIComponent(machineId)}/theme`, { theme });
