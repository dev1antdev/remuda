// What a schedule's row says about its last run: a skipped one in the page's words, else the server's own result.

import type { Schedule } from "./api";
import { t } from "@/lib/i18n";

// resultWords is the last run's outcome as the row reads it
export function resultWords(s: Pick<Schedule, "skipped" | "last_result">): string {
  switch (s.skipped) {
    case "working":
      return t("skipped: the agent is still working");
    case "asking":
      return t("skipped: the agent waits for an answer");
    case "starting":
      return t("skipped: the agent has not started on the prompt yet");
    case "unsure":
      return t("skipped: no word yet from its machine");
    case "running":
      return t("skipped: the previous run is still on its way");
    case "limit":
      return t("skipped: its agent's plan window is nearly spent");
    default:
      return s.last_result ?? "";
  }
}

// resultTone is how the outcome is coloured: a skip quiet, a run that went green, a failure red
export const resultTone = (s: Pick<Schedule, "skipped" | "last_ok">): "skip" | "ok" | "bad" => (s.skipped ? "skip" : s.last_ok ? "ok" : "bad");
