// The concierge's server calls: where it runs and why it cannot, the person's machine, a start, a new conversation, a stop.

import { getJSON, sendJSON } from "@/lib/http";

// ConciergeMachine is a machine the concierge can run on, or the one picked when it cannot run there now
export type ConciergeMachine = { id: string; label: string; connected: boolean; mac?: boolean };

// ConciergeRunning is the concierge standing on its machine: its workspace, the pane it talks in, its folder
export type ConciergeRunning = { machine: string; workspace: string; pane: string; dir: string; started?: boolean; resumed?: boolean };

// ConciergeView is where the concierge runs (the pick, else the default), the picker's machines, and why it cannot run now
export type ConciergeView = { machine: string; picked?: string; machines: ConciergeMachine[]; trouble?: string; running?: ConciergeRunning };

export const fetchConcierge = (): Promise<ConciergeView> => getJSON("/api/concierge");

// pickConcierge keeps the person's machine, "" for the default; the next start moves the concierge there
export const pickConcierge = (machine: string): Promise<ConciergeView> => sendJSON("PUT", "/api/concierge", { machine });

// startConcierge is the concierge on its machine, started there when it is not running; nothing is sent to it
export const startConcierge = (): Promise<ConciergeRunning> => sendJSON("POST", "/api/concierge", {});

// newConversation ends the conversation and starts the concierge afresh
export const newConversation = (): Promise<ConciergeRunning> => sendJSON("POST", "/api/concierge/new", {});

// stopConcierge closes its workspace; the conversation comes back on the next start
export const stopConcierge = (): Promise<unknown> => sendJSON("DELETE", "/api/concierge", {});
