// The live ports as the status bar shows them, after Orca's live ports: how many the workspaces hold, the popup's
// groups by workspace in the sidebar's order with the rest folded as other processes, and the words of a stop. The pure part.

import type { ListeningPort, Session } from "@/features/roster/api";
import { t, tn } from "@/lib/i18n";

// MachinePort is a port as GET /api/ports lists it: with the workspace whose folder holds its process, "" for none
export type MachinePort = ListeningPort & { workspace?: string };

export type PortGroup = { session: Session; ports: MachinePort[] };

// workspacePorts counts the ports the machine's workspaces hold, as the roster carries them
export const workspacePorts = (sessions: Session[]): number => sessions.reduce((n, s) => n + (s.ports?.length ?? 0), 0);

// portGroups lays a machine's ports out by workspace in the roster's order; a port of no workspace the roster lists
// (remuda's own, one outside every folder) goes with the others
export function portGroups(sessions: Session[], ports: MachinePort[]): { groups: PortGroup[]; other: MachinePort[] } {
  const groups: PortGroup[] = [];
  const placed = new Set<MachinePort>();
  for (const session of sessions) {
    const mine = ports.filter((p) => p.workspace === session.id);
    if (!mine.length) continue;
    mine.forEach((p) => placed.add(p));
    groups.push({ session, ports: [...mine].sort((a, b) => a.port - b.port) });
  }
  const other = ports.filter((p) => !placed.has(p)).sort((a, b) => a.port - b.port);
  return { groups, other };
}

// processWords is what listens: the server's name with its program, "vite (node)", else the program
export const processWords = (p: ListeningPort): string => (p.label && p.label !== p.process ? `${p.label} (${p.process})` : p.process || t("PID {pid}", { pid: p.pid }));

// portsTip is the segment's tooltip: the workspaces' ports and the others
export const portsTip = (inWorkspaces: number, other: number | null): string =>
  other === null ? tn(inWorkspaces, "{n} port in the workspaces", "{n} ports in the workspaces") : t("{workspaces} · {other}", { workspaces: tn(inWorkspaces, "{n} port in the workspaces", "{n} ports in the workspaces"), other: tn(other, "{n} other", "{n} others") });

// stopAsk is the confirm before a stop, naming the process, its pid, the port and the machine
export const stopAsk = (p: ListeningPort, machine: string): string =>
  t("Stop {process} (PID {pid}), listening on port {port} of {machine}? It gets SIGTERM, the way Ctrl+C in its terminal would end it; whatever it was serving goes with it.", { process: processWords(p), pid: p.pid, port: p.port, machine });

// stoppedWords is the toast after a stop
export const stoppedWords = (p: ListeningPort): string => t("{process} on port {port} was told to stop", { process: processWords(p), port: p.port });
