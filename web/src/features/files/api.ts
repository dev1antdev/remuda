// Files the agent wrote: resolved, read, diffed, uploaded — on the machine the pane lives on.

import { check, getJSON } from "@/lib/http";

export type FileInfo = {
  path: string;
  kind: "text" | "image" | "media" | "pdf" | "binary" | "dir" | "missing";
  size: number;
  candidates?: string[];
};

export type FileDiff = { repo?: string; untracked?: boolean; diff: string };

export const fsStat = (machineId: string, paneId: string, path: string): Promise<FileInfo> => getJSON(`/api/fs/stat?${new URLSearchParams({ machine: machineId, pane: paneId, path })}`);

export type DirListing = { path: string; parent?: string; home?: string; dirs: { name: string; git: boolean }[] };

// the subfolders of a folder on a machine ("" is home), for the picker
export const listDirs = (machineId: string, path: string, hidden: boolean): Promise<DirListing> =>
  getJSON(`/api/fs/dirs?${new URLSearchParams({ machine: machineId, path, hidden: hidden ? "1" : "0" })}`);

export function fileUrl(machineId: string, path: string): string {
  return `/api/fs/file?${new URLSearchParams({ machine: machineId, path })}`;
}

export async function fsText(machineId: string, path: string): Promise<{ text: string; truncated: boolean; max: number }> {
  const res = await check(await fetch(fileUrl(machineId, path)));
  return {
    text: await res.text(),
    truncated: res.headers.get("X-Remuda-Truncated") === "true",
    max: Number(res.headers.get("X-Remuda-Max") ?? 0),
  };
}

export type Upload = { name: string; path: string; rel: string; size: number; kind: string };

// uploadFiles stores files in the pane's folder (.remuda/uploads) and returns their paths; up to 4 a call, 25 MB each
export async function uploadFiles(machineId: string, paneId: string, files: File[]): Promise<Upload[]> {
  const form = new FormData();
  for (const f of files) form.append("file", f, f.name);
  const q = new URLSearchParams({ machine: machineId, pane: paneId });
  const res = await check(await fetch(`/api/fs/upload?${q}`, { method: "POST", body: form }));
  return ((await res.json()) as { files: Upload[] }).files;
}

export const fsDiff = (machineId: string, path: string): Promise<FileDiff> => getJSON(`/api/fs/diff?${new URLSearchParams({ machine: machineId, path })}`);
