// The terminal's reads: a pane's screen as text with its colours, for the wall's screen view.

import { check } from "@/lib/http";

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
