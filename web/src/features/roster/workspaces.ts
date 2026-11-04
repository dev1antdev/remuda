// The sidebar's list for one machine, after Orca: a line per workspace that opens its agent in one click, the
// worktrees of one repository gathered under the repository's name, and a small line per agent only when a
// workspace runs more than one. Pure functions over the roster; rows.tsx draws them.

import { basename } from "@/lib/text";
import { t, tn } from "@/lib/i18n";
import type { Machine, Session } from "./api";
import { groups, rowLabel, type AgentRow, type Group, type RadarPrefs } from "./radar";

// WorkspaceItem is one workspace's line: its agents in the radar's order, the one its line speaks for, and how lively it is
export type WorkspaceItem = { session: Session; rows: AgentRow[]; lead: AgentRow | null; best: number };

export type Entry =
  | { kind: "workspace"; item: WorkspaceItem }
  | { kind: "repo"; key: string; name: string; items: WorkspaceItem[] };

// itemOf is a workspace's line; its liveliness is its own agents', not its worktrees'
const itemOf = (g: Group): WorkspaceItem => ({ session: g.session, rows: g.rows, lead: leadOf(g.rows), best: g.rows.reduce((best, r) => Math.max(best, r.last), 0) });

// entries lays out a machine: by the radar's grouping, a repository with linked worktrees open becomes a group of its
// checkouts (the main one first), every other workspace a line of its own; the flat order drops the groups
export function entries(m: Machine, prefs: RadarPrefs): Entry[] {
  const families = groups(m, prefs);
  if (prefs.order === "recent") {
    const items = families.flatMap((g) => [g, ...g.children]).map(itemOf);
    const order = new Map(items.map((item, i) => [item, i]));
    items.sort((a, b) => b.best - a.best || order.get(a)! - order.get(b)!);
    return items.map((item) => ({ kind: "workspace", item }));
  }
  return families.map((g): Entry => {
    if (g.children.length === 0) return { kind: "workspace", item: itemOf(g) };
    const name = g.session.worktree?.repo_name || basename(g.session.dir || g.session.worktree?.checkout_path || "") || g.session.name;
    return { kind: "repo", key: `repo:${g.session.worktree?.repo_key ?? g.session.id}`, name, items: [g, ...g.children].map(itemOf) };
  });
}

// pressing is the order a workspace's line picks its agent in: a question, a watchdog note, work, a finish, the rest
function pressing(r: AgentRow): number {
  const radar = r.pane.radar;
  if (!r.pane.agent) return 5;
  if (radar?.state === "blocked") return 0;
  if (radar?.alert) return 1;
  if (radar?.state === "working") return 2;
  if (radar?.state === "done") return 3;
  return 4;
}

// leadOf is the agent a workspace's line stands for and a click opens: the most pressing, ties in the radar's order
export function leadOf(rows: AgentRow[]): AgentRow | null {
  let lead: AgentRow | null = null;
  for (const r of rows) if (!lead || pressing(r) < pressing(lead)) lead = r;
  return lead && lead.pane.agent ? lead : (rows[0] ?? null);
}

// itemTitle names the line: the workspace's name, or inside its repository's group, a checkout named as the
// repository goes by its branch
export function itemTitle(item: WorkspaceItem, group?: string): string {
  const name = item.session.name;
  if (group && name === group) return item.session.branch || t("main checkout");
  return name;
}

// branchShown is the branch for the second line, none when the title already says it
export function branchShown(item: WorkspaceItem, title: string): string {
  const branch = item.session.branch ?? "";
  return branch && branch !== title ? branch : "";
}

// agentWords is what one agent's line says after its mark: the watchdog's note, the question, the answer it finished
// with, else its title
export function agentWords(r: AgentRow, prefs: RadarPrefs): string {
  const radar = r.pane.radar;
  if (radar?.alert) return radar.alert;
  if (radar?.state === "blocked") return t("waiting for your answer");
  if ((radar?.state === "done" || radar?.state === "idle") && r.pane.said) return r.pane.said;
  return rowLabel(r.pane, r.tab, r.session.name, prefs);
}

// itemWords is the second line's text: the lead agent's words, how many agents run when more than one, or for a
// workspace of shells alone, its tabs
export function itemWords(item: WorkspaceItem, prefs: RadarPrefs): string {
  if (item.rows.length > 1) {
    const agents = item.rows.filter((r) => r.pane.agent).length;
    const shells = item.rows.length - agents;
    return [agents ? tn(agents, "{n} agent", "{n} agents") : "", shells ? tn(shells, "{n} shell", "{n} shells") : ""].filter(Boolean).join(" · ");
  }
  if (item.lead?.pane.agent) return agentWords(item.lead, prefs);
  const tabs = item.session.tabs.length;
  return tabs > 1 ? tn(tabs, "{n} tab", "{n} tabs") : t("terminal");
}

// since is the moment a line's age counts from: the start of the work or the question, else the last turn
export function since(r: AgentRow | null): number | undefined {
  const radar = r?.pane.radar;
  if (!radar) return undefined;
  if ((radar.state === "working" || radar.state === "blocked") && radar.since) return radar.since;
  return radar.last_active || undefined;
}

// shortAge is a line's age at a glance: "now", "12m", "3h", "4d"
export function shortAge(from: number | undefined, now: number): string {
  if (!from) return "";
  const minutes = Math.floor(Math.max(0, now - from) / 60_000);
  if (minutes < 1) return t("now");
  if (minutes < 60) return t("{n}m", { n: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return t("{n}h", { n: hours });
  return t("{n}d", { n: Math.floor(hours / 24) });
}

// representative is the pane a workspace without an agent opens on: the active tab's focused one, else its first
export function representative(s: Session) {
  const tab = s.tabs.find((x) => x.id === s.active_tab_id) ?? s.tabs[0];
  if (!tab) return null;
  return tab.panes.find((p) => p.focused) ?? tab.panes[0] ?? null;
}
