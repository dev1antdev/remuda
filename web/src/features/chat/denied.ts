// The pure part of the chat's notice on the calls Claude Code denied without asking, nobody being there to approve
// them (an agent remuda started with "Don't wait for permissions"): which to list, the words over them, and the
// dismissal, kept per conversation until another call is denied.

import type { Denial } from "@/features/agents/deny";
import { tn } from "@/lib/i18n";

export const MAX_SHOWN = 5;

// shownDenials is the calls the notice lists, in the order they were denied, and how many more there were
export function shownDenials(calls: Denial[]): { shown: Denial[]; more: number } {
  return { shown: calls.slice(0, MAX_SHOWN), more: Math.max(0, calls.length - MAX_SHOWN) };
}

// deniedKey is the pref the notice's dismissal is kept under: per machine and conversation
export const deniedKey = (machineId: string, session: string): string => `denied.seen:${machineId}|${session}`;

// lastDenial names the latest denied call, which a dismissal remembers
export const lastDenial = (calls: Denial[]): string => {
  const last = calls[calls.length - 1];
  return last ? last.tool_id || `${last.at ?? ""}|${last.tool}` : "";
};

// showsDenied says whether the notice stands: a call was denied, and not only the ones it was dismissed for
export const showsDenied = (calls: Denial[] | undefined, dismissed: string): boolean => !!calls?.length && lastDenial(calls) !== dismissed;

// deniedHead is the notice's first line
export const deniedHead = (n: number): string =>
  tn(n, "Claude Code was denied a call nobody was there to approve:", "Claude Code was denied {n} calls nobody was there to approve:");
