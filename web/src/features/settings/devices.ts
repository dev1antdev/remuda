// The words under a signed-in device: what it is when its name says something else, when it was last seen, where from.

import { ago } from "@/lib/time";
import type { Device } from "./api";
import { t } from "@/lib/i18n";

// deviceLine is "Chrome on macOS · seen 3 h ago · from 192.168.1.4", the kind left out when the name is the kind
export function deviceLine(device: Pick<Device, "name" | "kind" | "last_seen" | "ip">, now = Date.now()): string {
  const parts: string[] = [];
  if (device.kind && device.kind !== device.name) parts.push(device.kind);
  parts.push(t("seen {ago}", { ago: ago(device.last_seen, now) }));
  if (device.ip) parts.push(t("from {ip}", { ip: device.ip }));
  return parts.join(" · ");
}

// revokeQuestion is the confirm before a device is signed out
export function revokeQuestion(device: Pick<Device, "name" | "this">): { text: string; label: string } {
  if (device.this) return { text: t("Sign this device out? The token is needed to sign in again."), label: t("Sign out") };
  return { text: t("Sign “{device}” out? It needs the token to sign in again.", { device: device.name }), label: t("Revoke") };
}
