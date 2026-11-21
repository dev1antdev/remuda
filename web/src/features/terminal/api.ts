// The terminal's read: the find over its scrollback.

import { getJSON } from "@/lib/http";
import type { Found } from "./scrollback";

// findScrollback asks where the query occurs in the pane's last rows and where its viewport stands
export function findScrollback(machineId: string, paneId: string, query: string, lines: number): Promise<Found> {
  const q = new URLSearchParams({ machine: machineId, pane: paneId, q: query, lines: String(lines) });
  return getJSON<Found>(`/api/scrollback?${q}`);
}
