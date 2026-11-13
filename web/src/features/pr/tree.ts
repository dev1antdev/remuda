// The Files tab's tree and the agents on a pull request, as plain functions.

import type { Machine } from "@/features/roster/api";
import { folderOf } from "@/features/roster/workspaces";
import { basename } from "@/lib/text";
import type { PRFile } from "./api";

export type TreeRow = { kind: "folder"; name: string; path: string; depth: number } | { kind: "file"; file: PRFile; name: string; depth: number };

type Node = { dirs: Map<string, Node & { path: string }>; files: PRFile[] };

// fileTree lays the files out as folders first, then files, each folder's own rows left out while it is closed
export function fileTree(files: PRFile[], closed: Set<string> = new Set()): TreeRow[] {
  const root: Node = { dirs: new Map(), files: [] };
  for (const file of files) {
    const parts = file.path.split("/");
    parts.pop();
    let node = root;
    let path = "";
    for (const part of parts) {
      path = path ? `${path}/${part}` : part;
      if (!node.dirs.has(part)) node.dirs.set(part, { path, dirs: new Map(), files: [] });
      node = node.dirs.get(part)!;
    }
    node.files.push(file);
  }
  const rows: TreeRow[] = [];
  const walk = (node: Node, depth: number) => {
    for (const [name, dir] of [...node.dirs].sort(([a], [b]) => a.localeCompare(b))) {
      rows.push({ kind: "folder", name, path: dir.path, depth });
      if (!closed.has(dir.path)) walk(dir, depth + 1);
    }
    for (const file of node.files) rows.push({ kind: "file", file, name: basename(file.path), depth });
  };
  walk(root, 0);
  return rows;
}

// fileTotals is what the files change together
export const fileTotals = (files: PRFile[]) => files.reduce((sum, f) => ({ additions: sum.additions + f.additions, deletions: sum.deletions + f.deletions }), { additions: 0, deletions: 0 });

export type AgentOn = { machineId: string; paneId: string; agent: string; workspace: string; folder: string; state: string };

// agentsOn is every agent working on the pull request: one whose workspace on the panel's machine has its head branch
// checked out, or one anywhere that named the pull request itself
export function agentsOn(machines: Machine[], machineId: string, pr: { url: string; head: string }): AgentOn[] {
  const out: AgentOn[] = [];
  for (const m of machines)
    for (const s of m.sessions)
      for (const tab of s.tabs)
        for (const pane of tab.panes) {
          if (!pane.agent || !((m.id === machineId && s.branch === pr.head) || pane.pr?.url === pr.url)) continue;
          out.push({ machineId: m.id, paneId: pane.id, agent: pane.agent, workspace: s.name, folder: folderOf(s), state: pane.radar?.state ?? pane.status });
        }
  return out;
}
