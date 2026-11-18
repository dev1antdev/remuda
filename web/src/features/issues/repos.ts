// The repository picker: Follow the pane, the repositories picked lately (any machine), and the ones checked out on the
// panel's machine; a filter narrows them all. Which repository the panel is on is a pref of this device.

import { t } from "@/lib/i18n";
import { errMsg } from "@/lib/text";
import type { Repo } from "./api";

export type Picked = { machine: string; repo: string } | null; // null follows the pane
export const PICKED_KEY = "issues.picked";
export const RECENT_KEY = "issues.recent";
export const MAX_RECENT = 6;

export type Recent = { machine: string; repo: string };

// Placed is a read's outcome with where it was asked: kept on screen while the next one loads, it counts only for that
// place, since the rows' actions go to the place on screen by number. A failure is one too, so that a read overtaken by
// a later one is dropped with its failure; what the place showed before stays beside it.
export type Placed<T> = { place: string; answer?: T; failed?: string };
export const placeOf = (machine: string, repo = "") => `${machine}\n${repo.toLowerCase()}`;
export const answerAt = <T,>(placed: Placed<T> | null, place: string): T | undefined => (placed?.place === place ? placed.answer : undefined);
export const failureAt = (placed: Placed<unknown> | null, place: string): string => (placed?.place === place ? (placed.failed ?? "") : "");

// readAt runs a read for a place and answers with its outcome, a failure included; the place's last answer stays
export async function readAt<T>(place: string, last: Placed<T> | null, read: () => Promise<T>): Promise<Placed<T>> {
  try {
    return { place, answer: await read() };
  } catch (e) {
    return { place, answer: answerAt(last, place), failed: errMsg(e) };
  }
}

// remember puts a repository first among the recent ones, once
export function remember(recent: Recent[], entry: Recent): Recent[] {
  const same = (r: Recent) => r.machine === entry.machine && r.repo.toLowerCase() === entry.repo.toLowerCase();
  return [entry, ...recent.filter((r) => !same(r))].slice(0, MAX_RECENT);
}

export type PickRow =
  | { kind: "heading"; text: string }
  | { kind: "follow"; hint: string; checked: boolean }
  | { kind: "repo"; machine: string; repo: string; hint: string; checked: boolean };

// pickerRows lists what the picker offers; a filter leaves Follow the pane out, as it names no repository
export function pickerRows(o: { picked: Picked; machine: string; following: string; recent: Recent[]; repos: Repo[]; filter: string; machineLabel: (id: string) => string }): PickRow[] {
  const q = o.filter.trim().toLowerCase();
  const hit = (repo: string) => !q || repo.toLowerCase().includes(q);
  const seen = new Set<string>();
  const rows: PickRow[] = [];
  const isPicked = (machine: string, repo: string) => !!o.picked && o.picked.machine === machine && o.picked.repo.toLowerCase() === repo.toLowerCase();
  if (!q) rows.push({ kind: "follow", hint: o.following || t("No GitHub repository in the pane's folder"), checked: !o.picked });
  const section = (title: string, entries: Recent[]) => {
    const fresh = entries.filter((e) => hit(e.repo) && !seen.has(placeOf(e.machine, e.repo)));
    if (fresh.length === 0) return;
    rows.push({ kind: "heading", text: title });
    for (const e of fresh) {
      seen.add(placeOf(e.machine, e.repo));
      rows.push({ kind: "repo", machine: e.machine, repo: e.repo, hint: e.machine === o.machine ? "" : o.machineLabel(e.machine), checked: isPicked(e.machine, e.repo) });
    }
  };
  section(t("Recent"), o.recent);
  section(t("On {machine}", { machine: o.machineLabel(o.machine) }), o.repos.map((r) => ({ machine: o.machine, repo: r.repo })));
  return rows;
}
