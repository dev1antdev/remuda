// The sidebar's two other ways to gather a machine's workspaces, after munza's better-sidebar: by priority — sections
// for the ones that need you, the finished, the working and the idle — and by ticket prefix (PR-12, INS-4, also after
// a folder as in feature/INS-4), "No prefix" last. Pure; workspaces.ts lays them out.

import { t } from "@/lib/i18n";
import type { WorkspaceItem } from "./workspaces";

// Priority is a section of the priority order, the most urgent first: a question or a watchdog note, a finish, work, rest
export type Priority = "needs" | "done" | "working" | "idle";
export const PRIORITIES: Priority[] = ["needs", "done", "working", "idle"];

// Section is a heading of the sidebar with the workspaces under it: its key, its words, the state it is about when it
// gathers by priority
export type Section = { key: string; title: string; priority?: Priority; items: WorkspaceItem[] };

const RANK: Record<Priority, number> = { needs: 3, done: 2, working: 1, idle: 0 };

// priorityOf is the section a workspace goes in: its most urgent agent's, a parent taking its most urgent one's place
export function priorityOf(item: WorkspaceItem): Priority {
  let most: Priority = "idle";
  for (const r of item.rows) {
    const radar = r.pane.radar;
    if (!r.pane.agent || !radar) continue;
    const here: Priority = radar.state === "blocked" || radar.alert ? "needs" : radar.state === "done" ? "done" : radar.state === "working" ? "working" : "idle";
    if (RANK[here] > RANK[most]) most = here;
  }
  return most;
}

// PREFIX finds PR-12, INS-4 and PLAT-work, also after a folder as in feature/INS-4: capitals need only the dash, lower
// case needs a number after it, so pr-12 counts and herdr-mods does not
const PREFIX = /(?:^|\/)(?:([A-Z][A-Z0-9]*)-\S|([A-Za-z][A-Za-z0-9]*)-\d)/;

// ticketPrefix is the first of the names that carries a ticket's prefix, in capitals; null for none
export function ticketPrefix(...names: (string | undefined)[]): string | null {
  for (const name of names) {
    const match = PREFIX.exec(name ?? "");
    if (match) return (match[1] ?? match[2])!.toUpperCase();
  }
  return null;
}

// prefixOf reads a workspace's prefix off its name, then its branch, then the names given to its tabs
export function prefixOf(item: WorkspaceItem): string | null {
  const s = item.session;
  return ticketPrefix(s.name, s.branch, ...s.tabs.filter((tab) => tab.named).map((tab) => tab.label));
}

const PRIORITY_TITLE: Record<Priority, () => string> = {
  needs: () => t("Needs you"),
  done: () => t("Done"),
  working: () => t("Working"),
  idle: () => t("Idle"),
};

// byActivity puts the liveliest workspace first, ties in the order given
function byActivity(items: WorkspaceItem[]): WorkspaceItem[] {
  const order = new Map(items.map((item, i) => [item, i]));
  return [...items].sort((a, b) => b.best - a.best || order.get(a)! - order.get(b)!);
}

// prioritySections gathers the workspaces by the section their most urgent agent puts them in, each section by
// activity; an empty section is left out
export function prioritySections(items: WorkspaceItem[]): Section[] {
  return PRIORITIES.map((priority) => ({ key: `priority:${priority}`, title: PRIORITY_TITLE[priority](), priority, items: byActivity(items.filter((item) => priorityOf(item) === priority)) })).filter(
    (section) => section.items.length > 0,
  );
}

// prefixSections gathers the workspaces by their ticket's prefix, the prefixes in alphabetical order and the ones with
// none last, each section by activity
export function prefixSections(items: WorkspaceItem[]): Section[] {
  const gathered = new Map<string, WorkspaceItem[]>();
  for (const item of items) {
    const prefix = prefixOf(item) ?? "";
    gathered.set(prefix, [...(gathered.get(prefix) ?? []), item]);
  }
  const prefixes = [...gathered.keys()].sort((a, b) => Number(a === "") - Number(b === "") || a.localeCompare(b));
  return prefixes.map((prefix) => ({ key: `prefix:${prefix}`, title: prefix ? `${prefix}-` : t("No prefix"), items: byActivity(gathered.get(prefix)!) }));
}
