// Waking a sleeping machine: the pure part of the menu rows. A machine can be woken when it is away and has an
// address; the address is checked the way the server checks it, so a typo is refused before the request.

import type { Machine } from "./api";

const MAC = /^([0-9a-f]{2}[:-]){5}[0-9a-f]{2}$|^([0-9a-f]{4}\.){2}[0-9a-f]{4}$/i;

// canWake says whether the Wake up row applies: the machine is not connected and an address is known
export const canWake = (m: Pick<Machine, "state" | "mac">): boolean => m.state !== "connected" && !!m.mac;

// isMAC accepts the three spellings the server reads: aa:bb:cc:dd:ee:ff, aa-bb-cc-dd-ee-ff, aabb.ccdd.eeff
export const isMAC = (s: string): boolean => MAC.test(s.trim());

// wokeWords is the toast after the packet went out
export function wokeWords(label: string, sent: number): string {
  return sent > 0 ? `Woke ${label} — reconnecting` : `Nothing could carry the packet to ${label}`;
}
