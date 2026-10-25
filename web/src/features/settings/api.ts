// Settings: the signed-in devices this remuda keeps.

import { getJSON, sendJSON } from "@/lib/http";

export type Device = {
  id: string;
  name: string;
  kind: string; // the browser and the system its user agent names, "Chrome on macOS"
  created: string;
  last_seen: string;
  ip?: string;
  this: boolean; // the device this page runs on
};

export const fetchDevices = (): Promise<Device[]> => getJSON("/api/devices");
export const renameDevice = (id: string, name: string): Promise<Device> => sendJSON("PATCH", `/api/devices/${encodeURIComponent(id)}`, { name });
export const revokeDevice = (id: string): Promise<{ revoked: string; self: boolean }> => sendJSON("DELETE", `/api/devices/${encodeURIComponent(id)}`);
export const revokeOtherDevices = (): Promise<{ revoked: number }> => sendJSON("DELETE", "/api/devices?others=1");
// The shared settings on the server: every synced key with the moment it was set.

import { check, sendJSON } from "@/lib/http";
import type { Stamped } from "@/lib/syncKeys";

export type SharedPrefs = Record<string, Stamped>;

// fetchPrefs reads the server's copy; undefined means unchanged since the ETag given
export async function fetchPrefs(etag: string): Promise<{ prefs: SharedPrefs; etag: string } | undefined> {
  const res = await check(await fetch("/api/prefs", { headers: etag ? { "If-None-Match": etag } : undefined }));
  if (res.status === 304) return undefined;
  return { prefs: (await res.json()) as SharedPrefs, etag: res.headers.get("ETag") ?? "" };
}

// putPrefs merges these keys into the server's copy and answers the merged whole
export const putPrefs = (part: SharedPrefs) => sendJSON<SharedPrefs>("PUT", "/api/prefs", part);
