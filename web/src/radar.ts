// The sidebar's reading of every agent — after hhdebb/herdr-radar's design,
// written for this UI: the server holds the lifecycle (held done, held
// question, last activity, in internal/radar); this file decides what that
// looks like and in what order. Pure functions over the roster.

import type { Machine, Pane, Session, Tab } from "./api";

// ---- vendors ----

// Each vendor's own published hue, chosen to read on a dark panel; green and
// red stay out because they mean done and blocked. A vendor absent here has
// no hue of its own and its mark is drawn in ink.
export const BRAND: Record<string, string> = {
  claude: "#d97757",
  gemini: "#4285f4",
  kimi: "#1783ff",
  deepseek: "#4d6bfe",
  qwen: "#615ced",
  kiro: "#9046ff",
  cline: "#586876", // #323b43 lightened: as published it dies on a dark panel
  kilo: "#9a9808", // #f8f676 darkened: as published it dies on a light one
  kimchi: "#ff521d",
  muse: "#0082fb",
  crush: "#ff388b",
};

// What to call an agent whose own title never says.
export const VENDOR_NAME: Record<string, string> = {
  claude: "Claude Code",
  codex: "Codex",
  opencode: "OpenCode",
  omp: "Oh My Pi",
  cline: "Cline",
  mastracode: "Mastra",
  kimi: "Kimi",
  kilo: "Kilo",
  maki: "Maki",
  pi: "Pi",
  hermes: "Hermes",
  cursor: "Cursor",
  copilot: "Copilot",
  deepseek: "DeepSeek",
  gemini: "Gemini",
  gpt: "GPT",
  qwen: "Qwen",
  grok: "grok",
  agy: "Antigravity",
  kiro: "Kiro",
  amp: "Amp",
  devin: "Devin",
  qodercli: "Qoder",
  glm: "GLM",
  kimchi: "Kimchi",
  muse: "Muse",
  crush: "Crush",
};

// the hue a working row and its mark take: the vendor's, else a recognised
// harness with no hue of its own
export const OTHER_HUE = "#c78a1f";
export const brandColor = (agent: string): string => BRAND[agent] ?? OTHER_HUE;

// ---- state ----

export type RadarState = "working" | "blocked" | "done" | "idle" | "unknown";
export type Tier = "fresh" | "normal" | "stale";
export type Display = RadarState | "idle_fresh" | "idle_stale";

export type RadarPrefs = {
  order: "active" | "recent" | "herdr";
  rowLabel: "title" | "tab" | "both";
  freshMinutes: number; // after the last turn, a pane still reads as fresh
  staleMinutes: number; // without a turn, the row dims
  shells: boolean; // list shell panes too
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

// The mark in front of a title. Shape carries the state so the row still
// reads without colour: motion for working, a pulsing question when it
// waits on you, a tick for done (held until seen), a ring for idle.
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

// What names a row: the session's own title, the tab's name, or both. A
// title that only names the folder (a shell-filled terminal title) gives
// way to the vendor's name; one that repeats the workspace's name under a
// header that already shows it loses that prefix.
export function rowLabel(p: Pane, tab: Tab, workspace: string, prefs: RadarPrefs): string {
  let title = p.title.trim();
  const folder = p.cwd.split("/").filter(Boolean).pop() ?? "";
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

// ---- order ----

// `last` is the row's activity for ordering: a working row is the liveliest
// of all (its stamp is quantized by the server), the rest their last turn,
// no stamp at all = 0. `index` is herdr's own order, the tie-breaker.
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

// Within a workspace: the tabs by their liveliest pane, a split's first pane
// before the rest, then each pane by its own activity; ties keep herdr's order.
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

// A machine's workspaces as families — a repository with its worktrees under
// it — ordered by the liveliest agent in the family, the parent first and
// its children by their own; families with no agent keep herdr's order at
// the end.
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
