// The status bar's server calls: the machine's load as its herdr panes make it, its listening ports as last read,
// and a port's process stopped at the person's word.

import type { ListeningPort } from "@/features/roster/api";
import { getJSON, sendJSON, withMachine } from "@/lib/http";
import type { Usage } from "./load";
import type { MachinePort } from "./ports";

export const fetchLoad = (machineId: string): Promise<Usage> => getJSON(withMachine("/api/resources", machineId));

// PortsReading is the machine's ports as last read: read says whether they were read at all yet, unsupported why
// they never are (a Windows machine), error why the last read failed — the ports before it stand
export type PortsReading = { at?: string; ports: MachinePort[]; read: boolean; error?: string; unsupported?: string };

export const fetchPorts = (machineId: string): Promise<PortsReading> => getJSON(withMachine("/api/ports", machineId));

export const stopPort = (machineId: string, port: ListeningPort): Promise<{ stopped: unknown }> => sendJSON("POST", "/api/ports/stop", { machine: machineId, pid: port.pid, port: port.port });
