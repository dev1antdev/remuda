// The activity panel's filters: the kinds the feed carries, grouped into the families a person thinks in, and the
// machine; pure, tested. The pref `activity.filter` keeps them per device.

import type { IconName } from "@/ui";
import type { Entry } from "./api";

export type Filter = { kinds: string[]; machine: string }; // the families on (none = every kind); "all" or a machine id
export const DEFAULT_FILTER: Filter = { kinds: [], machine: "all" };

// Family is one chip: the English label (a t() key), its icon and the feed kinds it gathers
export type Family = { key: string; label: string; icon: IconName; kinds: string[] };

export const OTHER = "other";

// FAMILIES is the chip row in its order; "other" takes a handover, a recovery, a test and any kind not named here
export const FAMILIES: Family[] = [
  { key: "questions", label: "Questions", icon: "alert-triangle", kinds: ["input", "question"] },
  { key: "finished", label: "Finished", icon: "circle-check", kinds: ["done", "finish"] },
  { key: "watchdog", label: "Watchdog", icon: "clock", kinds: ["stuck", "waiting", "loop"] },
  { key: "tasks", label: "Tasks", icon: "list", kinds: ["task"] },
  { key: "schedules", label: "Schedules", icon: "calendar", kinds: ["schedule", "digest"] },
  { key: "budgets", label: "Budgets", icon: "chart", kinds: ["budget"] },
  { key: "review", label: "Review", icon: "eye", kinds: ["review"] },
  { key: "machines", label: "Machines", icon: "monitor", kinds: ["machine"] },
  { key: "sent", label: "Sent", icon: "send", kinds: ["sent"] },
  { key: OTHER, label: "Other", icon: "bell", kinds: [] },
];

// familyOf is the chip an entry's kind belongs to
export function familyOf(kind: string): string {
  return FAMILIES.find((family) => family.kinds.includes(kind))?.key ?? OTHER;
}

// isFiltering says whether anything is left out
export const isFiltering = (filter: Filter): boolean => filter.kinds.length > 0 || filter.machine !== "all";

// filterEntries keeps the entries of the families on (every family when none is) from the machine chosen
export function filterEntries(entries: Entry[], filter: Filter): Entry[] {
  return entries.filter((entry) => (filter.machine === "all" || entry.machine === filter.machine) && (filter.kinds.length === 0 || filter.kinds.includes(familyOf(entry.kind))));
}

// counts is how many entries of the chosen machine each family has, so the chips say what they would show
export function counts(entries: Entry[], machine: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const entry of entries) {
    if (machine !== "all" && entry.machine !== machine) continue;
    const key = familyOf(entry.kind);
    out[key] = (out[key] ?? 0) + 1;
  }
  return out;
}

// toggleFamily turns one chip on or off
export function toggleFamily(filter: Filter, key: string): Filter {
  const kinds = filter.kinds.includes(key) ? filter.kinds.filter((k) => k !== key) : [...filter.kinds, key];
  return { ...filter, kinds };
}

// filterWords says what is on, for the bell's tooltip: the families' labels (translated by the caller) and the machine
export function filterWords(filter: Filter, label: (family: Family) => string, machineLabel: string): string {
  const parts: string[] = [];
  const families = FAMILIES.filter((family) => filter.kinds.includes(family.key)).map(label);
  if (families.length > 0) parts.push(families.join(", "));
  if (filter.machine !== "all") parts.push(machineLabel);
  return parts.join(" · ");
}
