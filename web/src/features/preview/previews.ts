// The previews dialog's pure part: what a port is called, which ports are the workspace's and which the machine's
// others, and whether a port is open on the cockpit already.

import { basename } from "@/lib/text";
import type { Listener, Preview } from "./api";

// listenerWords is a port as the dialog names it: "3000 · node · vite"
export const listenerWords = (l: Listener): string => [String(l.port), l.command, l.label].filter(Boolean).join(" · ");

// whereWords is the folder a port's server runs in, by its last name
export const whereWords = (l: Listener): string => (l.dir ? basename(l.dir) : "");

// split sorts the ports by where their servers run: the workspace's, the machine's other workspaces', and the rest —
// an app of the machine's own, a browser's debugging port — which the dialog folds away; each by port
export function split(listeners: Listener[], workspace?: string): { here: Listener[]; workspaces: Listener[]; others: Listener[] } {
  const byPort = (a: Listener, b: Listener) => a.port - b.port;
  return {
    here: workspace ? listeners.filter((l) => l.workspace === workspace).sort(byPort) : [],
    workspaces: listeners.filter((l) => l.workspace && l.workspace !== workspace).sort(byPort),
    others: listeners.filter((l) => !l.workspace).sort(byPort),
  };
}

// openOn is the preview of a port already open on the cockpit, if there is one
export const openOn = (open: Preview[], port: number): Preview | undefined => open.find((p) => p.port === port);

// hostOf is a preview's address without its scheme, for the row
export function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}
