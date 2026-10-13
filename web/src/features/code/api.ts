// The Code mode: whether a machine's code-server answers.

import { getJSON } from "@/lib/http";

export type CodeStatus = { available: boolean; port: number; error?: string };

// whether the machine's code-server answers (force re-probes a cached "no")
export const codeStatus = (machineId: string, force = false): Promise<CodeStatus> => getJSON(`/api/code/${encodeURIComponent(machineId)}${force ? "?check=1" : ""}`);
