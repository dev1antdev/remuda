// The backups' server calls: the settings and the last runs, a run now, the copies on the target.

import { getJSON, sendJSON } from "@/lib/http";

export type BackupSettings = { machine: string; at: string; keep: number };
export type BackupResult = { at: string; machine: string; label?: string; path?: string; size?: number; files?: number; took: number; pruned?: number; error?: string; manual?: boolean };
export type BackupStatus = { settings: BackupSettings; last?: BackupResult; last_good?: BackupResult; next?: string; running: boolean; host: string };
export type BackupFile = { name: string; size: number; at: string };

export const fetchBackups = (): Promise<BackupStatus> => getJSON("/api/backups");
export const saveBackups = (settings: BackupSettings): Promise<BackupStatus> => sendJSON("PUT", "/api/backups", settings);
export const runBackup = (): Promise<BackupResult> => sendJSON("POST", "/api/backups/run");
export const listBackups = (): Promise<{ folder: string; files: BackupFile[] }> => getJSON("/api/backups/files");
