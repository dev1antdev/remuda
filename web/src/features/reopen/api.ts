// The agents whose panes went away, kept by the server to be brought back on their own sessions.

import { getJSON, sendJSON } from "@/lib/http";

export type Closed = {
  id: string;
  machine: string;
  pane: string;
  agent: string;
  session: string;
  dir: string;
  title: string;
  workspace?: string;
  tab?: string; // the tab's own name, when it had one
  at: string; // RFC 3339
};

export const listClosed = (): Promise<Closed[]> => getJSON<{ closed: Closed[] }>("/api/closed").then((r) => r.closed ?? []);

// reopenClosed starts the agent again on its session; the answer names its pane, or the pane that runs it already
export const reopenClosed = (machine: string, id: string): Promise<{ pane: string; workspace: string; open_already?: boolean }> => sendJSON("POST", "/api/closed/reopen", { machine, id });
