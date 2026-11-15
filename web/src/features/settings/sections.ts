// The sections of Settings: which ones the dialog lists, in two groups — what this device keeps and what the cockpit
// keeps for every device — which one is on screen, and where an arrow key takes the list.

import { t } from "@/lib/i18n";
import type { IconName } from "@/ui/icons";

export const SECTION_IDS = ["look", "sidebar", "general", "sleep", "herdr", "jira", "backups", "timelapse", "devices"] as const;
export type SectionId = (typeof SECTION_IDS)[number];

export type Section = { id: SectionId; label: string; icon: IconName };
export type SectionGroup = { id: "device" | "cockpit"; label: string; sections: Section[] };

// SECTION_PREF keeps the section last shown, per device
export const SECTION_PREF = "settings.section";
export const FIRST_SECTION: SectionId = "look";

// sectionGroups is the list as the dialog draws it; a viewer keeps this device's sections alone
export function sectionGroups(viewOnly: boolean): SectionGroup[] {
  const device: SectionGroup = {
    id: "device",
    label: t("This device"),
    sections: [
      { id: "look", label: t("Look"), icon: "palette" },
      { id: "sidebar", label: t("Sidebar"), icon: "columns" },
      { id: "general", label: t("General"), icon: "sliders" },
    ],
  };
  if (viewOnly) return [device];
  const cockpit: SectionGroup = {
    id: "cockpit",
    label: t("This cockpit"),
    sections: [
      { id: "sleep", label: t("Sleep and resume"), icon: "moon" },
      { id: "herdr", label: t("Inside herdr"), icon: "square-terminal" },
      { id: "jira", label: t("Jira"), icon: "tag" },
      { id: "backups", label: t("Backups"), icon: "archive" },
      { id: "timelapse", label: t("Timelapse"), icon: "film" },
      { id: "devices", label: t("Devices"), icon: "monitor" },
    ],
  };
  return [device, cockpit];
}

// listedIds is every section listed, in the list's order
export function listedIds(groups: SectionGroup[]): SectionId[] {
  return groups.flatMap((group) => group.sections.map((section) => section.id));
}

// shownSection is the section to draw for what was asked or kept: that one while the list has it, else the first
export function shownSection(groups: SectionGroup[], asked: unknown): SectionId {
  const ids = listedIds(groups);
  return ids.find((id) => id === asked) ?? ids[0] ?? FIRST_SECTION;
}

// stepSection is where a key takes the list from the section in the focus: the arrows a step either way, round and
// round, Home and End to its ends; null for any other key
export function stepSection(ids: SectionId[], from: SectionId, key: string): SectionId | null {
  const at = ids.indexOf(from);
  if (at < 0) return null;
  const to = key === "ArrowDown" || key === "ArrowRight" ? at + 1 : key === "ArrowUp" || key === "ArrowLeft" ? at - 1 : key === "Home" ? 0 : key === "End" ? ids.length - 1 : null;
  return to === null ? null : (ids[(to + ids.length) % ids.length] ?? null);
}
