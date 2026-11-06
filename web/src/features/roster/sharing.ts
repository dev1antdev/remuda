// Screen-sharing mode, the pure part: which folders and machines the person hides while their screen is shared, the
// neutral names those read as ("Project A", "Machine 2"), and the hidden names swapped out of any text that names them.
// A page-only switch, per device: nothing the server keeps or sends changes.

import { t } from "@/lib/i18n";

export type SharingPref = { on: boolean; all: boolean; folders: string[]; machines: string[] };

export const SHARING_KEY = "sharing";
export const DEFAULT_SHARING: SharingPref = { on: false, all: false, folders: [], machines: [] };

// folderKey names a folder of a machine in the marks
export const folderKey = (machineId: string, folder: string) => `${machineId}|${folder}`;

// sharingOf reads the pref as stored, any damage read as the switch off and nothing marked
export function sharingOf(value: unknown): SharingPref {
  if (!value || typeof value !== "object") return DEFAULT_SHARING;
  const v = value as Partial<SharingPref>;
  const strings = (list: unknown) => (Array.isArray(list) ? list.filter((x): x is string => typeof x === "string") : []);
  return { on: v.on === true, all: v.all === true, folders: strings(v.folders), machines: strings(v.machines) };
}

export const hidesMachine = (p: SharingPref, machineId: string) => p.on && (p.all || p.machines.includes(machineId));

export const hidesFolder = (p: SharingPref, machineId: string, folder: string) => p.on && (p.all || p.machines.includes(machineId) || p.folders.includes(folderKey(machineId, folder)));

// markedFolder and markedMachine say whether a folder or machine is on the person's list, whether or not the mode is on
export const markedFolder = (p: SharingPref, machineId: string, folder: string) => p.folders.includes(folderKey(machineId, folder));
export const markedMachine = (p: SharingPref, machineId: string) => p.machines.includes(machineId);

export function toggleMark(list: string[], key: string): string[] {
  return list.includes(key) ? list.filter((x) => x !== key) : [...list, key];
}

// switched turns the mode on or off; on with nothing marked hides everything, so the switch never shows what it should hide
export function switched(p: SharingPref, on: boolean): SharingPref {
  if (!on) return { ...p, on: false };
  return { ...p, on: true, all: p.all || (p.folders.length === 0 && p.machines.length === 0) };
}

// letters counts A … Z, AA, AB …, as a spreadsheet names its columns
export function letters(n: number): string {
  let out = "";
  for (let i = n + 1; i > 0; i = Math.floor((i - 1) / 26)) out = String.fromCharCode(65 + ((i - 1) % 26)) + out;
  return out;
}

export const projectName = (n: number) => t("Project {letter}", { letter: letters(n) });
export const machineName = (n: number) => t("Machine {n}", { n: n + 1 });

// extendNames keeps every name already given and gives the keys seen for the first time the next ones, so a folder keeps
// its letter while the mode is on however the roster reorders; the same map comes back when nothing is new
export function extendNames(names: Map<string, string>, keys: string[], name: (n: number) => string): Map<string, string> {
  const fresh = keys.filter((key, i) => !names.has(key) && keys.indexOf(key) === i);
  if (fresh.length === 0) return names;
  const next = new Map(names);
  for (const key of fresh) next.set(key, name(next.size));
  return next;
}

// MIN_REDACTED is the shortest name swapped in free text: a shorter one would hit inside ordinary words
export const MIN_REDACTED = 3;

// redact swaps every hidden name in a text for its neutral one, the longest first, so "user-profiles" goes before "user"
export function redact(text: string, pairs: [string, string][]): string {
  const sorted = pairs.filter(([real, neutral]) => real.length >= MIN_REDACTED && real !== neutral).sort((a, b) => b[0].length - a[0].length);
  let out = text;
  for (const [real, neutral] of sorted) if (out.includes(real)) out = out.split(real).join(neutral);
  return out;
}

export const hiddenWords = () => t("hidden while sharing");
