// The machine's load as the status bar says it: "CPU 12 % · 3.8 GB", what its herdr panes weigh on it, amber and red
// where Orca's resources mod puts them — CPU from 30 and 60 % of all the cores, memory from 30 and 50 % of the RAM —
// and each workspace's line in the popup. The pure part.

import type { Session } from "@/features/roster/api";
import { locale, t, tn } from "@/lib/i18n";

// Weight is a tree of processes as the server sums it: CPU as a share of all the cores, resident memory, its heaviest
export type Weight = { cpu: number; rss_kb: number; top?: { name: string; rss_kb: number } };
export type Terminal = Weight & { pane: string };
export type WorkspaceLoad = Weight & { id: string; terminals: Terminal[] };

// Usage is what GET /api/resources answers: the machine's panes by workspace, herdr's own beside them, the sum
export type Usage = { at: string; cores: number; mem_kb: number; total: Weight; cpu_known: boolean; workspaces: WorkspaceLoad[]; herdr: Weight; unread?: number };

export type Level = "" | "amber" | "red";

export const cpuLevel = (share: number): Level => (share >= 60 ? "red" : share >= 30 ? "amber" : "");

export function memLevel(kb: number, totalKB: number): Level {
  if (!totalKB) return "";
  if (kb >= totalKB * 0.5) return "red";
  if (kb >= totalKB * 0.3) return "amber";
  return "";
}

// graver is the worse of two levels
export const graver = (a: Level, b: Level): Level => (a === "red" || b === "red" ? "red" : a === "amber" || b === "amber" ? "amber" : "");

const number = (n: number, digits: number) => n.toLocaleString(locale(), { minimumFractionDigits: digits, maximumFractionDigits: digits });

// cpuWords is a CPU share in a few characters: "0 %", "4.2 %", "12 %"
export function cpuWords(share: number): string {
  if (share < 0.05) return t("{n} %", { n: 0 });
  return t("{n} %", { n: share < 10 ? number(share, 1) : Math.round(share).toString() });
}

// memWords is resident memory: "512 MB", "3.8 GB"
export function memWords(kb: number): string {
  if (kb >= 1024 * 1024) return t("{n} GB", { n: number(kb / 1024 / 1024, 1) });
  return t("{n} MB", { n: Math.round(kb / 1024) });
}

// barWords is the bar's segment: "CPU 12 %", the CPU still being read as "CPU …"
export const cpuBarWords = (u: Usage): string => t("CPU {cpu}", { cpu: u.cpu_known ? cpuWords(u.total.cpu) : "…" });

// terminalsWords counts a workspace's terminals
export const terminalsWords = (n: number): string => tn(n, "{n} terminal", "{n} terminals");

// workspaceLine is a workspace's line under its name: its heaviest process and its terminals, "node 512 MB · 3 terminals"
export function workspaceLine(ws: WorkspaceLoad): string {
  const count = terminalsWords(ws.terminals.length);
  return ws.top ? `${ws.top.name} ${memWords(ws.top.rss_kb)} · ${count}` : count;
}

// footWords is the popup's foot: what the CPU share means, and the machine's memory
export const footWords = (u: Usage): string => t("CPU is a share of all {cores} cores · {mem}", { cores: u.cores, mem: memWords(u.mem_kb) });

// loadTip is the segment's tooltip: whose load it is and against what
export function loadTip(u: Usage, machine: string): string {
  const cpu = u.cpu_known ? cpuWords(u.total.cpu) : "…";
  return t("herdr's panes on {machine}: CPU {cpu} of {cores} cores · {mem} of {total}", { machine, cpu, cores: u.cores, mem: memWords(u.total.rss_kb), total: memWords(u.mem_kb) });
}

// terminalName is a terminal's line in the popup: what runs in the pane — its agent, else a shell — and its heaviest
// process when that is another program, "Claude Code · node"
export function terminalName(session: Session | undefined, term: Terminal): string {
  const pane = session?.tabs.flatMap((tab) => tab.panes).find((p) => p.id === term.pane);
  const what = pane?.agent || t("terminal");
  const top = term.top?.name;
  return top && top.toLowerCase() !== what.toLowerCase() ? `${what} · ${top}` : what;
}

// paneToOpen is the pane a click on a workspace opens: an agent of its active tab, else any agent, else its first pane
export function paneToOpen(session: Session): string | undefined {
  const active = session.tabs.find((tab) => tab.id === session.active_tab_id);
  const agentOf = (panes: Session["tabs"][number]["panes"]) => panes.find((p) => p.agent)?.id;
  return (active && agentOf(active.panes)) ?? session.tabs.map((tab) => agentOf(tab.panes)).find(Boolean) ?? (active ?? session.tabs[0])?.panes[0]?.id;
}
