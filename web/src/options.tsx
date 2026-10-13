// Select options built from the roster, the same way everywhere: machines
// with their state dot, workspace folders with their name and path.

import type { Machine } from "./api";
import Icon from "./icons";
import type { ReactNode } from "react";

export type Option<T> = { value: T; label: string; hint?: string; group?: string; icon?: ReactNode; disabled?: boolean };

export const machineLabel = (machines: Machine[], id: string) => machines.find((m) => m.id === id)?.label ?? id;

export const connected = (machines: Machine[]) => machines.filter((m) => m.state === "connected");

export function machineOptions(machines: Machine[], opts: { all?: string; onlyConnected?: boolean } = {}): Option<string>[] {
  const list = opts.onlyConnected === false ? machines : connected(machines);
  const out: Option<string>[] = list.map((m) => ({ value: m.id, label: m.label, icon: <span className={`dot machine-state is-${m.state}`} /> }));
  if (opts.all) out.unshift({ value: "all", label: opts.all });
  return out;
}

// folderOptions lists the workspace folders the roster knows on one machine or all; the hint is the
// folder's path, or on "all" the machines it is on.
export function folderOptions(machines: Machine[], machineId: string | "all"): Option<string>[] {
  const seen = new Map<string, { name: string; machines: string[] }>();
  for (const m of machines) {
    if (machineId !== "all" && m.id !== machineId) continue;
    for (const s of m.sessions) {
      if (!s.dir) continue;
      const cur = seen.get(s.dir);
      if (cur) cur.machines.push(m.label);
      else seen.set(s.dir, { name: s.name, machines: [m.label] });
    }
  }
  const several = machineId === "all" && machines.length > 1;
  return [...seen.entries()]
    .sort((a, b) => a[1].name.localeCompare(b[1].name))
    .map(([dir, v]) => ({ value: dir, label: v.name, hint: several ? v.machines.join(", ") : dir, icon: <Icon name="folder" size={14} /> }));
}
