// Fetch, pull and push of one checkout at the person's press: the calls the Repositories place makes, for any other
// screen with a checkout to offer them on (the Git lens). Each answers what it came to, a refusal or git's failure
// included; words for it are outcome.ts's.

import { sendJSON } from "@/lib/http";
import type { SecretScan } from "@/features/review/secrets";
import type { Repo } from "./api";

export type SyncAction = "fetch" | "pull" | "push";

// Outcome is what one action came to (internal/gitsync's Outcome)
export type Outcome = {
  action: SyncAction;
  state: "done" | "refused" | "failed" | "held";
  reason?: string; // why it was refused or held, or what a done one came to: up-to-date, new-branch
  agent?: string; // the kind of agent a pull was refused for
  doing?: "working" | "asking" | "ending";
  remote?: string;
  branch?: string;
  upstream?: string;
  ahead?: number;
  behind?: number;
  count?: number; // the commits a pull brought or a push sent
  output?: string; // git's last lines when it failed, or why the machine is away
  scan?: SecretScan; // what the secret check found, for a held push
  repo?: Repo; // the checkout read again after
};

export const fetchRepo = (machine: string, dir: string): Promise<Outcome> => sendJSON("POST", "/api/repos/fetch", { machine, dir });

// pullRepo fast-forwards only, and is refused while an agent works, asks or ends a turn in the checkout
export const pullRepo = (machine: string, dir: string): Promise<Outcome> => sendJSON("POST", "/api/repos/pull", { machine, dir });

// pushRepo never forces; the secret check holds it back ("held") until anyway says so, after the person confirmed
export const pushRepo = (machine: string, dir: string, anyway = false): Promise<Outcome> => sendJSON("POST", "/api/repos/push", { machine, dir, anyway });

// syncRepo is any of the three by name
export function syncRepo(action: SyncAction, machine: string, dir: string, anyway = false): Promise<Outcome> {
  if (action === "fetch") return fetchRepo(machine, dir);
  if (action === "pull") return pullRepo(machine, dir);
  return pushRepo(machine, dir, anyway);
}
