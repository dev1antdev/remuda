// The previews' server calls: a machine's listening ports matched to its workspaces, one opened on the cockpit, one shut.

import { getJSON, sendJSON, withMachine } from "@/lib/http";

// Listener is a port a process of the person's listens on, with the workspace whose folder holds it ("" for none)
export type Listener = { port: number; address: string; pid: number; command: string; label?: string; dir?: string; workspace?: string };

// Preview is a machine's port open on the cockpit, at url
export type Preview = { machine: string; port: number; local: number; url: string; opened: string };

export const fetchPreviews = (machineId: string): Promise<{ listeners: Listener[]; open: Preview[] }> => getJSON(withMachine("/api/previews", machineId));

export const openPreview = (machineId: string, port: number): Promise<Preview> => sendJSON("POST", "/api/previews/open", { machine: machineId, port });

export const closePreview = (machineId: string, port: number): Promise<{ closed: boolean }> => sendJSON("DELETE", "/api/previews", { machine: machineId, port });
