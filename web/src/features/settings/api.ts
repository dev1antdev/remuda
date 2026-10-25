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
