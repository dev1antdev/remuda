// The floating terminal's shell on the server: one plain-shell workspace per machine, made the first time it is asked for.

import { sendJSON } from "@/lib/http";

export type ScratchShell = { workspace: string; pane: string; made: boolean };

// openScratch is the machine's scratch shell, made in its home folder when it has none
export const openScratch = (machine: string): Promise<ScratchShell> => sendJSON("POST", "/api/scratch", { machine });

// closeScratch ends the machine's scratch shell; the next open makes a fresh one
export const closeScratch = (machine: string): Promise<unknown> => sendJSON("DELETE", "/api/scratch", { machine });
