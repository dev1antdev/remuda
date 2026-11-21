// The closed agents as the page says them, which of them a close just made, and the key that lists them. The pure
// part; ReopenDialog and offer.ts use it.

import { VENDOR_NAME } from "@/ui/brand";
import { t, tn } from "@/lib/i18n";
import type { Closed } from "./api";

// REOPEN_KEY is Alt+Z, by key code: ⌘⇧T and Ctrl+Shift+T are the browser's own reopen
export const REOPEN_KEY = "Alt+Z";

// closedTitle names a closed agent: its conversation's title, else the agent's name
export function closedTitle(c: Closed): string {
  return c.title.trim() || VENDOR_NAME[c.agent] || c.agent;
}

// closedWhere is where it lived: its tab's name, its workspace, its machine when that is not this one
export function closedWhere(c: Closed, machineLabel: string): string {
  return [c.tab, c.workspace, c.machine !== "local" ? machineLabel : ""].filter(Boolean).join(" · ");
}

// justClosed is what a close made of the panes it closed: their entries on that machine, newest first
export function justClosed(list: Closed[], machine: string, panes: string[]): Closed[] {
  const wanted = new Set(panes);
  return list.filter((c) => c.machine === machine && wanted.has(c.pane));
}

// offerWords is the toast's line after a close
export function offerWords(closed: Closed[]): string {
  if (closed.length === 1) return t("Closed {agent}", { agent: closedTitle(closed[0]!) });
  return tn(closed.length, "Closed {n} agent", "Closed {n} agents");
}

export type KeyPress = {
  code: string;
  altKey: boolean;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  target: { closest?: (selector: string) => unknown } | null;
};

const FIELDS = "input, textarea, select, [contenteditable]";

// reopenKeyAsked says whether a press asks for the closed agents: Alt+Z by key code, never over the terminal, which
// keeps its own keys, nor over a dialog; on a Mac never over a field either, where Option+Z types a character
export function reopenKeyAsked(press: KeyPress, dialogOpen: boolean, mac: boolean): boolean {
  if (press.code !== "KeyZ" || !press.altKey || press.metaKey || press.ctrlKey || press.shiftKey) return false;
  if (dialogOpen || press.target?.closest?.(".xterm")) return false;
  return !(mac && press.target?.closest?.(FIELDS));
}
