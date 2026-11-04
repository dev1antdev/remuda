// useSharing is the screen-sharing mode as every screen reads it: whether a machine or a folder is hidden, the neutral
// name it reads as, and the hidden names swapped out of a text. The switch and the marks live in the pref `sharing`, per
// device; the functions below the hook change them from a menu, the palette or a key.

import { useMemo } from "react";
import { readPref, usePref, writePref } from "@/lib/prefs";
import { basename } from "@/lib/text";
import type { Machine, Session } from "./api";
import { useRoster } from "./useRoster";
import { DEFAULT_RADAR, type RadarPrefs } from "./radar";
import { entries, folderOf } from "./workspaces";
import {
  DEFAULT_SHARING,
  SHARING_KEY,
  extendNames,
  folderKey,
  hidesFolder,
  hidesMachine,
  machineName,
  projectName,
  redact,
  sharingOf,
  switched,
  toggleMark,
  type SharingPref,
} from "./sharing";

export type Sharing = {
  pref: SharingPref;
  on: boolean;
  machineHidden: (machineId: string) => boolean;
  folderHidden: (machineId: string, folder: string) => boolean;
  sessionHidden: (machineId: string, session: Session) => boolean;
  machineLabel: (machine: Machine) => string;
  folderName: (machineId: string, folder: string, real: string) => string;
  redact: (text: string) => string;
};

// given keeps the neutral names handed out while the mode is on, so a folder keeps its letter across renders and screens
let given = { machines: new Map<string, string>(), folders: new Map<string, string>() };

const OPEN: Omit<Sharing, "pref"> = {
  on: false,
  machineHidden: () => false,
  folderHidden: () => false,
  sessionHidden: () => false,
  machineLabel: (machine) => machine.label,
  folderName: (_machineId, _folder, real) => real,
  redact: (text) => text,
};

export function useSharing(): Sharing {
  const [stored] = usePref<SharingPref>(SHARING_KEY, DEFAULT_SHARING);
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const { machines } = useRoster();
  return useMemo(() => {
    const pref = sharingOf(stored);
    if (!pref.on) {
      given = { machines: new Map(), folders: new Map() };
      return { ...OPEN, pref };
    }
    const machineKeys = machines.filter((m) => hidesMachine(pref, m.id)).map((m) => m.id);
    // the letters go out in the sidebar's order, so the first folder on screen is Project A
    const folderKeys: string[] = [];
    for (const m of machines)
      for (const entry of entries(m, radar))
        for (const item of entry.kind === "folder" ? entry.folder.items : [entry.item]) {
          const folder = folderOf(item.session);
          if (folder && hidesFolder(pref, m.id, folder)) folderKeys.push(folderKey(m.id, folder));
        }
    given = { machines: extendNames(given.machines, machineKeys, machineName), folders: extendNames(given.folders, folderKeys, projectName) };
    const names = given;
    const pairs: [string, string][] = [];
    for (const m of machines) {
      const neutral = names.machines.get(m.id);
      if (neutral) {
        pairs.push([m.label, neutral]);
        if (m.host) pairs.push([m.host, neutral]);
      }
      for (const s of m.sessions) {
        const folder = folderOf(s);
        const name = folder ? names.folders.get(folderKey(m.id, folder)) : undefined;
        if (!name) continue;
        pairs.push([basename(folder), name], [s.name, name], [folder, name]);
        if (s.branch) pairs.push([s.branch, "•••"]);
      }
    }
    const folderHidden = (machineId: string, folder: string) => hidesFolder(pref, machineId, folder);
    return {
      pref,
      on: true,
      machineHidden: (machineId: string) => hidesMachine(pref, machineId),
      folderHidden,
      sessionHidden: (machineId: string, session: Session) => folderHidden(machineId, folderOf(session)),
      machineLabel: (machine: Machine) => names.machines.get(machine.id) ?? machine.label,
      folderName: (machineId: string, folder: string, real: string) => (folderHidden(machineId, folder) ? (names.folders.get(folderKey(machineId, folder)) ?? projectName(names.folders.size)) : real),
      redact: (text: string) => redact(text, pairs),
    };
  }, [stored, machines, radar]);
}

const current = () => sharingOf(readPref<SharingPref>(SHARING_KEY, DEFAULT_SHARING));

// toggleSharing turns the mode on or off; on with nothing marked hides everything
export const toggleSharing = () => writePref(SHARING_KEY, switched(current(), !current().on));

// setSharingAll picks between hiding everything and hiding only what was marked
export const setSharingAll = (all: boolean) => writePref(SHARING_KEY, { ...current(), all });

export const toggleSharingFolder = (machineId: string, folder: string) => {
  const p = current();
  writePref(SHARING_KEY, { ...p, folders: toggleMark(p.folders, folderKey(machineId, folder)) });
};

export const toggleSharingMachine = (machineId: string) => {
  const p = current();
  writePref(SHARING_KEY, { ...p, machines: toggleMark(p.machines, machineId) });
};
