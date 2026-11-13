// The workspace card's own calls: its note kept or removed, and its pull requests read again while its hover card shows.

import { sendJSON } from "@/lib/http";

// saveNote keeps a checkout's note; an empty text removes it. The answer is the text as the server keeps it.
export const saveNote = (machine: string, dir: string, text: string): Promise<{ text: string }> => sendJSON("PUT", "/api/notes", { machine, dir, text });

// refreshPRs asks gh on the machine for the requests of the panes given again, each whose last read is over 20 s old
export const refreshPRs = (machine: string, panes: string[]): Promise<{ reading: number }> => sendJSON("POST", "/api/pr/refresh", { machine, panes });
