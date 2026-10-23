// The sidebar's reading of every agent, after hhdebb/herdr-radar's design:
// the server holds the lifecycle (internal/radar), this file decides what
// it looks like and in what order. Pure functions over the roster.

import type { Machine, Pane, Session, Tab } from "@/features/roster/api";
import { basename } from "@/lib/text";
import { ago, elapsed } from "@/lib/time";
import { VENDOR_NAME } from "@/ui/brand";

// ---- state ----

// the server holds done until the pane is seen and a question until the agent works again; the idle tiers below are the UI's, from last_active and the clock
export type RadarState = "working" | "blocked" | "done" | "idle" | "unknown";
export type Tier = "fresh" | "normal" | "stale";
export type Display = RadarState | "idle_fresh" | "idle_stale";

export type RadarPrefs = {
  order: "active" | "recent" | "herdr";
  rowLabel: "title" | "tab" | "both";
  freshMinutes: number;
  staleMinutes: number;
  shells: boolean;
};

export const DEFAULT_RADAR: RadarPrefs = { order: "active", rowLabel: "title", freshMinutes: 15, staleMinutes: 120, shells: false };

// Idle cools with the time since the last turn; no stamp reads as plain idle.
export function tier(lastActive: number | undefined, prefs: RadarPrefs, now: number): Tier {
  if (!lastActive) return "normal";
  const age = now - lastActive;
  if (age <= prefs.freshMinutes * 60000) return "fresh";
  if (age >= prefs.staleMinutes * 60000) return "stale";
  return "normal";
}

export function displayOf(p: Pane, prefs: RadarPrefs, now: number): Display {
  const r = p.radar;
  if (!r) return "idle";
  if (r.state !== "idle") return r.state;
  const t = tier(r.last_active, prefs, now);
  return t === "fresh" ? "idle_fresh" : t === "stale" ? "idle_stale" : "idle";
}

// the braille spinner: eight dots, the gap walking round
export const FRAMES = ["⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽", "⣾"];
export const SPIN_MS = 150;
export const PULSE_STEPS = 5; // spinner steps per half of the blocked pulse

// markOf is the mark in front of a title; its shape carries the state, so a row reads without colour: motion working, a pulsing ? blocked, a tick done (held until seen), a ring idle
export function markOf(d: Display, step: number): string {
  switch (d) {
    case "working":
      return FRAMES[step % FRAMES.length]!;
    case "blocked":
      return Math.floor(step / PULSE_STEPS) % 2 === 1 ? "◦" : "?";
    case "done":
      return "✓";
    case "unknown":
      return "◌";
    default:
      return "◦";
  }
}

// ---- labels ----

// rowLabel names a row: the session's title, the tab's name or both; a title that only names the folder gives way to the vendor's name, one repeating the workspace's name loses that prefix
export function rowLabel(p: Pane, tab: Tab, workspace: string, prefs: RadarPrefs): string {
  let title = p.title.trim();
  const folder = p.cwd ? basename(p.cwd) : "";
  if (!title || title === p.cwd || title === folder || title.startsWith(`${p.cwd}: `)) title = VENDOR_NAME[p.agent] || p.agent || folder || "shell";
  if (workspace && title.length > workspace.length + 1) {
    const rest = title.slice(workspace.length);
    if (title.startsWith(workspace) && /^[\s:·\-–—/|]+/.test(rest)) title = rest.replace(/^[\s:·\-–—/|]+/, "");
  }
  const tabName = tab.named ? tab.label : "";
  if (prefs.rowLabel === "tab") return tabName || title;
  if (prefs.rowLabel === "both" && tabName) return `${tabName} · ${title}`;
  return title;
}

// stateWords says what an agent is doing, for a title or a row's right edge: the state, and for a resting one when its last turn was.
export function stateWords(d: Display, last: number | undefined, now: number, since?: number): string {
  const when = last ? ` · last turn ${ago(last, now)}` : "";
  const how = since ? ` for ${elapsed(since, now)}` : "";
  switch (d) {
    case "working":
      return `working${how}`;
    case "blocked":
      return `waiting on you${how}`;
    case "done":
      return `done${when}`;
    case "idle_fresh":
      return `idle, just stopped${when}`;
    case "idle_stale":
      return `idle for a long time${when}`;
    case "unknown":
      return "state unknown";
    default:
      return `idle${when}`;
  }
}

// ---- order ----

// AgentRow: `last` orders the rows — a working row is the liveliest of all (the server quantizes its stamp), the rest their last turn, no stamp = 0; `index` is herdr's own order, the tie-breaker
export type AgentRow = { session: Session; tab: Tab; pane: Pane; last: number; head: boolean; index: number };

export function agentRows(s: Session, prefs: RadarPrefs): AgentRow[] {
  const out: AgentRow[] = [];
  for (const tab of s.tabs) {
    let first = true;
    for (const pane of tab.panes) {
      if (!pane.agent && !prefs.shells) continue;
      const last = pane.radar?.state === "working" ? Number.MAX_SAFE_INTEGER : (pane.radar?.last_active ?? 0);
      out.push({ session: s, tab, pane, last, head: first, index: out.length });
      first = false;
    }
  }
  return out;
}

// orderRows, within a workspace: the tabs by their liveliest pane, a split's first pane before the rest, then each pane by its own activity; ties keep herdr's order
export function orderRows(rows: AgentRow[]): AgentRow[] {
  const tabBest = new Map<string, number>();
  const tabFirst = new Map<string, number>();
  for (const r of rows) {
    tabBest.set(r.tab.id, Math.max(tabBest.get(r.tab.id) ?? 0, r.last));
    if (!tabFirst.has(r.tab.id)) tabFirst.set(r.tab.id, r.index);
  }
  return [...rows].sort((a, b) => {
    const ta = tabBest.get(a.tab.id)!;
    const tb = tabBest.get(b.tab.id)!;
    if (ta !== tb) return tb - ta;
    if (a.tab.id !== b.tab.id) return tabFirst.get(a.tab.id)! - tabFirst.get(b.tab.id)!;
    if (a.head !== b.head) return a.head ? -1 : 1;
    return b.last - a.last || a.index - b.index;
  });
}

export type Group = { session: Session; rows: AgentRow[]; children: Group[]; best: number };

// groups is a machine's workspaces as families (a repository with its worktrees under it), by the liveliest agent in each, the parent first; families with no agent keep herdr's order at the end
export function groups(m: Machine, prefs: RadarPrefs): Group[] {
  const parentByRepo = new Map<string, Session>();
  for (const s of m.sessions) {
    if (s.worktree && !s.worktree.linked && !parentByRepo.has(s.worktree.repo_key)) parentByRepo.set(s.worktree.repo_key, s);
  }
  const childrenOf = new Map<string, Session[]>();
  const top: Session[] = [];
  for (const s of m.sessions) {
    const parent = s.worktree?.linked ? parentByRepo.get(s.worktree.repo_key) : undefined;
    if (parent && parent !== s) childrenOf.set(parent.id, [...(childrenOf.get(parent.id) ?? []), s]);
    else top.push(s);
  }
  const make = (s: Session): Group => {
    const rows = prefs.order === "active" ? orderRows(agentRows(s, prefs)) : agentRows(s, prefs);
    const children = (childrenOf.get(s.id) ?? []).map(make);
    const own = rows.reduce((a, r) => Math.max(a, r.last), 0);
    return { session: s, rows, children, best: Math.max(own, ...children.map((c) => c.best)) };
  };
  const out = top.map(make);
  if (prefs.order === "active") {
    const byActivity = (a: Group, b: Group) => b.best - a.best;
    out.sort((a, b) => (a.best && b.best ? byActivity(a, b) : a.best ? -1 : b.best ? 1 : 0));
    for (const g of out) g.children.sort(byActivity);
  }
  return out;
}

// The flat list: every agent on the machine by its last turn.
export function recentRows(m: Machine, prefs: RadarPrefs): AgentRow[] {
  const rows = m.sessions.flatMap((s) => agentRows(s, prefs));
  const order = new Map(rows.map((r, i) => [r, i]));
  return rows.sort((a, b) => b.last - a.last || order.get(a)! - order.get(b)!);
}

// A machine folded shut still says what is going on inside.
export function summary(m: Machine): { working: number; blocked: number; done: number } {
  const n = { working: 0, blocked: 0, done: 0 };
  for (const s of m.sessions)
    for (const t of s.tabs)
      for (const p of t.panes) {
        const st = p.radar?.state;
        if (st === "working" || st === "blocked" || st === "done") n[st]++;
      }
  return n;
}
