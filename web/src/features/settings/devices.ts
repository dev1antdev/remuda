// The words under a signed-in device: what it is when its name says something else, when it was last seen, where from.

import { ago } from "@/lib/time";
import type { Device } from "./api";

// deviceLine is "Chrome on macOS · seen 3 h ago · from 192.168.1.4", the kind left out when the name is the kind
export function deviceLine(device: Pick<Device, "name" | "kind" | "last_seen" | "ip">, now = Date.now()): string {
  const parts: string[] = [];
  if (device.kind && device.kind !== device.name) parts.push(device.kind);
  parts.push(`seen ${ago(device.last_seen, now)}`);
  if (device.ip) parts.push(`from ${device.ip}`);
  return parts.join(" · ");
}

// revokeQuestion is the confirm before a device is signed out
export function revokeQuestion(device: Pick<Device, "name" | "this">): { text: string; label: string } {
  if (device.this) return { text: "Sign this device out? The token is needed to sign in again.", label: "Sign out" };
  return { text: `Sign “${device.name}” out? It needs the token to sign in again.`, label: "Revoke" };
}
