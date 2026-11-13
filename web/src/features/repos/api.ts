// The Repositories place's reads: every machine's repositories, and the folder each machine scans for more.

import { getJSON, sendJSON } from "@/lib/http";

// Repo is one checkout as its row shows it (internal/gitsync's Repo)
export type Repo = {
  path: string;
  name: string;
  parent: string; // the folder it sits in, the home folder as ~
  workspace?: boolean; // a workspace stands on it; else the scan found it
  branch?: string;
  detached?: boolean;
  upstream?: string; // origin/main
  gone?: boolean; // an upstream is set but no longer there
  ahead: number;
  behind: number;
  dirty: number;
  staged?: number;
  conflicts?: number;
  remote: boolean;
  fetched?: string; // FETCH_HEAD's time
  error?: string; // git status's own line when it failed
  initial?: boolean; // no commit yet
};

// Scan is a folder of the machine searched for repositories, and how deep
export type Scan = { folder: string; depth: number };

// Listed is one machine's part of the place
export type Listed = {
  id: string;
  label: string;
  away?: string; // not connected: why ("connecting" before its first answer)
  error?: string; // the read failed
  scan?: Scan;
  scan_where?: string; // the scan's folder with the home said as ~
  scan_missing?: boolean;
  more?: boolean; // the scan found more than it lists
  repos: Repo[];
  at?: string;
};

export const listRepos = (fresh = false): Promise<{ machines: Listed[] }> => getJSON(`/api/repos${fresh ? "?fresh=1" : ""}`);

// setScan keeps the machine's folder to scan; an empty folder stops the scan
export const setScan = (machine: string, folder: string, depth: number): Promise<{ scan: Scan | null }> => sendJSON("PUT", "/api/repos/scan", { machine, folder, depth });
