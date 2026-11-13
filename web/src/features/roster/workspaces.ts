// The sidebar's list for one machine, after Orca: every repository a folder that folds shut, its workspaces under it
// as cards that open their agent in one click, and a line per agent under each card; or, in the orders that gather
// otherwise, sections by priority or by ticket prefix. Pure functions over the roster; rows.tsx draws them.

import { basename } from "@/lib/text";
import { t, tn } from "@/lib/i18n";
import type { FolderIconRef, Machine, Session } from "./api";
import { agentRows, orderRows, rowLabel, type AgentRow, type RadarPrefs } from "./radar";
import { prefixSections, prioritySections, type Section } from "./sections";

// WorkspaceItem is one workspace's card: its agents in the radar's order, the one its card speaks for, and how lively it is
export type WorkspaceItem = { session: Session; rows: AgentRow[]; lead: AgentRow | null; best: number };

// Folder is a repository — or a folder outside git — with every workspace open on it: the main checkout's and its
// worktrees', as Orca shows a project
export type Folder = { key: string; path: string; name: string; icon?: FolderIconRef; items: WorkspaceItem[]; best: number };

export type Entry = { kind: "workspace"; item: WorkspaceItem } | { kind: "folder"; folder: Folder } | { kind: "section"; section: Section };

// entryItems is every workspace an entry holds
export const entryItems = (entry: Entry): WorkspaceItem[] => (entry.kind === "folder" ? entry.folder.items : entry.kind === "section" ? entry.section.items : [entry.item]);

// foldKey is the key an entry's fold is kept under in the pref wsFolded, none for a lone workspace
export const foldKey = (machineId: string, entry: Entry): string | null =>
  entry.kind === "folder" ? `${machineId}|folder:${entry.folder.key}` : entry.kind === "section" ? `${machineId}|section:${entry.section.key}` : null;

// folderOf is the folder a workspace belongs to: the server's word (the repository's main folder), else its own
export const folderOf = (s: Session): string => s.folder || s.dir || s.worktree?.checkout_path || "";

// itemOf is a workspace's card; its liveliness is its own agents'
function itemOf(s: Session, prefs: RadarPrefs): WorkspaceItem {
  const rows = prefs.order === "recent" || prefs.order === "herdr" ? agentRows(s, prefs) : orderRows(agentRows(s, prefs));
  return { session: s, rows, lead: leadOf(rows), best: rows.reduce((best, r) => Math.max(best, r.last), 0) };
}

// entries lays out a machine: every folder with its workspaces under it, the main checkout first and the rest by
// activity, the folders by their liveliest agent (herdr's order for the ones with none, or with that order picked);
// the flat order lists the workspaces alone by activity; priority and prefix gather them in sections instead
export function entries(m: Machine, prefs: RadarPrefs): Entry[] {
  const items = m.sessions.filter((s) => !s.scratch).map((s) => itemOf(s, prefs)); // the floating terminal's shell is the drawer's
  if (prefs.order === "priority") return prioritySections(items).map((section) => ({ kind: "section", section }));
  if (prefs.order === "prefix") return prefixSections(items).map((section) => ({ kind: "section", section }));
  if (prefs.order === "recent") {
    const order = new Map(items.map((item, i) => [item, i]));
    return [...items].sort((a, b) => b.best - a.best || order.get(a)! - order.get(b)!).map((item) => ({ kind: "workspace", item }));
  }
  const folders = new Map<string, Folder>();
  for (const item of items) {
    const path = folderOf(item.session);
    const key = path || `workspace:${item.session.id}`;
    let folder = folders.get(key);
    if (!folder) {
      folder = { key, path, name: basename(path) || item.session.worktree?.repo_name || item.session.name, items: [], best: 0 };
      folders.set(key, folder);
    }
    folder.icon ??= item.session.icon;
    folder.items.push(item);
    folder.best = Math.max(folder.best, item.best);
  }
  const list = [...folders.values()];
  const main = (item: WorkspaceItem, folder: Folder) => !item.session.worktree?.linked && (item.session.dir || "") === folder.path;
  for (const folder of list) {
    const order = new Map(folder.items.map((item, i) => [item, i]));
    folder.items.sort((a, b) => Number(main(b, folder)) - Number(main(a, folder)) || (prefs.order === "active" ? b.best - a.best : 0) || order.get(a)! - order.get(b)!);
  }
  if (prefs.order === "active") {
    const order = new Map(list.map((folder, i) => [folder, i]));
    list.sort((a, b) => (a.best && b.best ? b.best - a.best : a.best ? -1 : b.best ? 1 : order.get(a)! - order.get(b)!));
  }
  return list.map((folder) => ({ kind: "folder", folder }));
}

// repositoryOf is the workspace a folder's new worktree branches off: its main checkout, else any of its workspaces in
// the repository; none outside git
export function repositoryOf(folder: Folder): Session | null {
  const inGit = folder.items.map((item) => item.session).filter((s) => s.worktree || s.branch);
  return inGit.find((s) => !s.worktree?.linked && (s.dir || "") === folder.path) ?? inGit[0] ?? null;
}

// folderCounts is what a folder's line says of its agents: how many work, ask and finished, and how many there are;
// the fresh files once per checkout, since two workspaces on one folder carry the same review
export function folderCounts(folder: Folder): { working: number; blocked: number; done: number; agents: number; fresh: number } {
  const n = { working: 0, blocked: 0, done: 0, agents: 0, fresh: 0 };
  const reviewed = new Set<string>();
  for (const item of folder.items) {
    const checkout = item.session.dir || `workspace:${item.session.id}`;
    if (!reviewed.has(checkout)) n.fresh += item.session.review?.fresh ?? 0;
    reviewed.add(checkout);
    for (const r of item.rows) {
      if (!r.pane.agent) continue;
      n.agents++;
      const state = r.pane.radar?.state;
      if (state === "working" || state === "blocked" || state === "done") n[state]++;
    }
  }
  return n;
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

// itemTitle names a card: the workspace's name, or inside its folder, a workspace named as the folder goes by its branch
export function itemTitle(item: WorkspaceItem, folder?: string): string {
  const name = item.session.name;
  if (folder && name === folder) return item.session.branch || name;
  return name;
}

// branchShown is the branch for the second line, none when the title already says it
export function branchShown(item: WorkspaceItem, title: string): string {
  const branch = item.session.branch ?? "";
  return branch && branch !== title ? branch : "";
}

// limited says whether the agent sits stopped on a plan limit: the hit stands until it works again
export const limited = (r: AgentRow | null): boolean => !!r?.pane.limit?.hit_at && r.pane.radar?.state !== "working";

// agentWords is what one agent's line says after its mark: the watchdog's note, the question, a plan limit it stopped
// on, the answer it finished with, else its title
export function agentWords(r: AgentRow, prefs: RadarPrefs): string {
  const radar = r.pane.radar;
  if (radar?.alert) return radar.alert;
  if (radar?.state === "blocked") return t("waiting for your answer");
  if (limited(r)) return r.pane.limit?.continue_at ? t("stopped on its plan limit · continues after the reset") : t("stopped on its plan limit");
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

// cardWords is a card's second line beside its branch, now that a line per agent names each: what the agent the card
// speaks for needs or said — a watchdog note, a question, a plan limit, its last answer — "" while it only works; a
// workspace of shells alone, its tabs
export function cardWords(item: WorkspaceItem): string {
  const lead = item.lead;
  if (!item.rows.some((r) => r.pane.agent)) {
    const tabs = item.session.tabs.length;
    return tabs > 1 ? tn(tabs, "{n} tab", "{n} tabs") : t("terminal");
  }
  const radar = lead?.pane.radar;
  if (radar?.alert) return radar.alert;
  if (radar?.state === "blocked") return t("waiting for your answer");
  if (limited(lead)) return lead?.pane.limit?.continue_at ? t("stopped on its plan limit · continues after the reset") : t("stopped on its plan limit");
  if ((radar?.state === "done" || radar?.state === "idle") && lead?.pane.said) return lead.pane.said;
  return "";
}

// primaryOf is the folder's main checkout when worktrees stand beside it, the card Orca marks "primary"; null otherwise
export function primaryOf(folder: Folder): WorkspaceItem | null {
  if (!folder.items.some((item) => item.session.worktree?.linked)) return null;
  return folder.items.find((item) => !item.session.worktree?.linked && (item.session.dir || "") === folder.path) ?? null;
}

// since is the moment a line's age counts from: the start of the work or the question, else the last turn
export function since(r: AgentRow | null): number | undefined {
  const radar = r?.pane.radar;
  if (!radar) return undefined;
  if ((radar.state === "working" || radar.state === "blocked") && radar.since) return radar.since;
  return radar.last_active || undefined;
}

// shortAge is a line's age at a glance: "<1m", "12m", "3h", "4d"
export function shortAge(from: number | undefined, now: number): string {
  if (!from) return "";
  const minutes = Math.floor(Math.max(0, now - from) / 60_000);
  if (minutes < 1) return t("<1m");
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
