// The View menu's way to the player, for the chat lens and the terminal lens alike.

import { actions } from "@/app/store";
import { t } from "@/lib/i18n";
import type { MenuItem } from "@/ui";

export type TimelapsePlace = { machineId: string; paneId: string; dir: string; agent: string; workspace: string; machine: string };

// timelapseTitle is what the player is headed with: the workspace, and its machine when that is not this one
export const timelapseTitle = (place: Pick<TimelapsePlace, "machineId" | "workspace" | "machine">): string =>
  place.machineId !== "local" && place.machine ? t("{workspace} on {machine}", { workspace: place.workspace, machine: place.machine }) : place.workspace;

// openTimelapse opens the player on the pane's recordings and its folder's
export function openTimelapse(place: TimelapsePlace) {
  actions.openDialog({ kind: "timelapse", machineId: place.machineId, paneId: place.paneId, dir: place.dir, title: timelapseTitle(place) });
}

// timelapseRows is the row that opens it: none to open for a shell or a transcript on its own
export function timelapseRows(place: TimelapsePlace): MenuItem[] {
  return [{ sep: true }, { label: t("Timelapse…"), icon: "film", hint: t("its turns as they looked on screen, played back in seconds; export as GIF"), disabled: !place.agent || !place.paneId, onSelect: () => openTimelapse(place) }];
}
