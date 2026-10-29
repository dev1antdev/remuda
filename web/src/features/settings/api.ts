// Settings' server calls: the signed-in devices this remuda keeps, and the shared settings on the server, every synced
// key with the moment it was set.

import { check, getJSON, sendJSON } from "@/lib/http";
import type { Stamped } from "@/lib/syncKeys";

export type Device = {
  id: string;
  name: string;
  kind: string; // the browser and the system its user agent names, "Chrome on macOS"
  created: string;
  last_seen: string;
  ip?: string;
  this: boolean; // the device this page runs on
  viewer?: boolean; // signed in by a viewer link: it reads and types nothing
};

export const fetchDevices = (): Promise<Device[]> => getJSON("/api/devices");
export const renameDevice = (id: string, name: string): Promise<Device> => sendJSON("PATCH", `/api/devices/${encodeURIComponent(id)}`, { name });
export const revokeDevice = (id: string): Promise<{ revoked: string; self: boolean }> => sendJSON("DELETE", `/api/devices/${encodeURIComponent(id)}`);
export const revokeOtherDevices = (): Promise<{ revoked: number }> => sendJSON("DELETE", "/api/devices?others=1");

// AgentKey is a machine's tasks-only key as the page sees it: never the key itself
export type AgentKey = {
  id: string;
  name: string; // the machine it was placed on
  scope: string; // "tasks"
  created: string;
  last_used: string;
};

export const fetchKeys = (): Promise<AgentKey[]> => getJSON("/api/agent-keys");
export const revokeKey = (id: string): Promise<{ revoked: string }> => sendJSON("DELETE", `/api/agent-keys/${encodeURIComponent(id)}`);

// ViewerKey is a viewer link as the page sees it: never the key itself, which the mint answers once
export type ViewerKey = {
  id: string;
  name: string; // whose screen it is, in the person's words
  created: string;
  last_used: string;
};

export const fetchViewerKeys = (): Promise<ViewerKey[]> => getJSON("/api/viewer-keys");
export const makeViewerKey = (name: string): Promise<{ id: string; name: string; created: string; link: string }> => sendJSON("POST", "/api/viewer-keys", { name });
export const revokeViewerKey = (id: string): Promise<{ revoked: string; screens: number }> => sendJSON("DELETE", `/api/viewer-keys/${encodeURIComponent(id)}`);


export type SharedPrefs = Record<string, Stamped>;

// fetchPrefs reads the server's copy; undefined means unchanged since the ETag given
export async function fetchPrefs(etag: string): Promise<{ prefs: SharedPrefs; etag: string } | undefined> {
  const res = await check(await fetch("/api/prefs", { headers: etag ? { "If-None-Match": etag } : undefined }));
  if (res.status === 304) return undefined;
  return { prefs: (await res.json()) as SharedPrefs, etag: res.headers.get("ETag") ?? "" };
}

// putPrefs merges these keys into the server's copy and answers the merged whole
export const putPrefs = (part: SharedPrefs) => sendJSON<SharedPrefs>("PUT", "/api/prefs", part);
