// The terminal's reads: a pane's screen as text with its colours for the wall's screen view, the find over its
// scrollback, and the one move, a scroll of the pane to a match.

import { check, getJSON, sendJSON } from "@/lib/http";
import type { Found, Scroll } from "./scrollback";

// findScrollback asks where the query occurs in the pane's last rows and where its viewport stands
export function findScrollback(machineId: string, paneId: string, query: string, lines: number): Promise<Found> {
  const q = new URLSearchParams({ machine: machineId, pane: paneId, q: query, lines: String(lines) });
  return getJSON<Found>(`/api/scrollback?${q}`);
}

// scrollPane moves the pane's viewport — every viewer's, as the wheel does — to that many rows above the bottom
export function scrollPane(machineId: string, paneId: string, offset: number): Promise<{ scroll: Scroll | null }> {
  return sendJSON("POST", "/api/scroll", { machine: machineId, pane: paneId, offset });
}

type ScreenResponse = { text: string; truncated: boolean };

// fetchScreen returns null when unchanged (304 via ETag).
export async function fetchScreen(machineId: string, paneId: string, etag: string | null): Promise<{ text: string; etag: string | null } | null> {
  const q = new URLSearchParams({ machine: machineId, pane: paneId });
  const res = await fetch(`/api/screen?${q}`, { headers: etag ? { "If-None-Match": etag } : {} });
  if (res.status === 304) return null;
  await check(res);
  const body = (await res.json()) as ScreenResponse;
  return { text: body.text, etag: res.headers.get("ETag") };
}
