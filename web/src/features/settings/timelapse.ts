// What the Timelapse section of Settings says the folder holds. Pure.

import { t, tn } from "@/lib/i18n";
import type { TimelapseStats } from "@/features/terminal/timelapse/api";
import { bytesWords } from "./backup";

// keptWords is how many recordings take how much of the most the folder holds
export function keptWords(stats: TimelapseStats): string {
  const max = bytesWords(stats.max);
  if (stats.count === 0) return t("Nothing kept yet; at most {max}, the oldest going first", { max });
  return tn(stats.count, "{n} recording, {size} of at most {max}; the oldest go first", "{n} recordings, {size} of at most {max}; the oldest go first", { size: bytesWords(stats.bytes), max });
}
