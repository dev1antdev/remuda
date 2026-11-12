// What the player says of a recording: its row in the picker, how the turn ended, the speeds' names, the GIF's name,
// and which recordings the screen-sharing mode leaves out. Pure.

import { t } from "@/lib/i18n";
import { spent, whenShort } from "@/lib/time";
import { VENDOR_NAME } from "@/ui/brand";
import type { RecordingMeta } from "./api";
import type { Speed } from "./playback";

export const durationOf = (meta: Pick<RecordingMeta, "start" | "end">): number => Math.max(0, Date.parse(meta.end) - Date.parse(meta.start));

// recordingLabel is a recording's row: when, how long, the agent, and — for an earlier pane of the folder — which
export function recordingLabel(meta: RecordingMeta, paneId: string): string {
  const parts = [whenShort(meta.start), spent(durationOf(meta) / 1000), VENDOR_NAME[meta.agent] ?? meta.agent];
  if (meta.pane !== paneId) parts.push(t("pane {pane}", { pane: meta.pane }));
  return parts.join(" · ");
}

// endedWords says how the turn ended
export function endedWords(ended: string): string {
  switch (ended) {
    case "done":
      return t("finished");
    case "idle":
      return t("came to rest");
    case "blocked":
      return t("asked you something");
    case "gone":
      return t("its pane closed");
    case "stopped":
      return t("remuda stopped while it worked");
    case "cut":
      return t("cut short: remuda went down while it worked");
    case "away":
      return t("cut short: its machine went out of reach while it worked");
  }
  return ended;
}

export const speedWords = (speed: Speed): string => (speed === "frame" ? t("By frame") : `${speed}×`);

// gifName is the file the export saves: the workspace, the day and time it started, the speed
export function gifName(meta: RecordingMeta, speed: Speed): string {
  const at = new Date(meta.start);
  const two = (n: number) => String(n).padStart(2, "0");
  const stamp = `${at.getFullYear()}-${two(at.getMonth() + 1)}-${two(at.getDate())}-${two(at.getHours())}${two(at.getMinutes())}`;
  const place = (meta.workspace || "timelapse").replace(/[^\p{L}\p{N}._-]+/gu, "-").replace(/^[-.]+|[-.]+$/g, "") || "timelapse";
  return `${place}-${stamp}-${speed === "frame" ? "frames" : `${speed}x`}.gif`;
}

// shownRecordings leaves out what the screen-sharing mode hides: a hidden machine's, a hidden folder's
export function shownRecordings(list: RecordingMeta[], hidden: { machine: (id: string) => boolean; folder: (machineId: string, folder: string) => boolean }): RecordingMeta[] {
  return list.filter((meta) => !hidden.machine(meta.machine) && !hidden.folder(meta.machine, meta.folder || meta.dir));
}
