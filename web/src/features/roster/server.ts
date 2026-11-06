// The host of a remuda serving the page only (remuda -server): no machine of the fleet, so no sidebar block, tile or
// picker row — but its vitals stand behind the Server place, and its own updates (a release) are run on it as on any
// machine, so it is given the machine's shape the Updates dialog takes.

import type { Machine, ServerHost } from "./api";

// hostMachine is the server's host as a machine with no workspaces, always connected: it is the one serving the page
export function hostMachine(server: ServerHost): Machine {
  return { id: server.id, label: server.label, state: "connected", sessions: [], health: server.health };
}

// startOn is the machine a "start an agent" offers first: this one when it is in the fleet, else the first connected, else the first
export function startOn(machines: Machine[]): string {
  const local = machines.find((m) => m.id === "local");
  return (local ?? machines.find((m) => m.state === "connected") ?? machines[0])?.id ?? "local";
}

// NO_MACHINES is what a server's page says while it has no machine to show: where the agents run is added with the
// sidebar's + or with herdr's own machine list on the server
export const NO_MACHINES = "This remuda serves the page only: add a machine its agents run on — the + beside Machines, any SSH host with herdr — or run herdr machine add on the server.";
