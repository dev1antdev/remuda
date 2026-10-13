// Full-text search over every transcript on every machine.

import { getJSON } from "@/lib/http";

export type SearchHit = { machine: string; project: string; dir: string; session: string; uuid: string; ts: string; role: "user" | "assistant"; snippet: string };
// encodePath is the name Claude Code gives a folder's project directory, the same rule as transcript.EncodePath on the server
export const encodePath = (dir: string) => dir.replace(/[^A-Za-z0-9]/g, "-");
export type SearchResult = { hits: SearchHit[]; errors?: Record<string, string>; more: boolean };
export const searchTranscripts = (q: string, f: { machine?: string; dir?: string; days?: number }, signal?: AbortSignal): Promise<SearchResult> => {
  const p = new URLSearchParams({ q });
  if (f.machine) p.set("machine", f.machine);
  if (f.dir) p.set("dir", f.dir);
  if (f.days) p.set("days", String(f.days));
  return getJSON(`/api/search?${p}`, { signal });
};
