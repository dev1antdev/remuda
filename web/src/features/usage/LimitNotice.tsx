// LimitNotice is the chat's word on an agent a plan limit stopped: when, until when, and the person's own booking to
// say "continue" at the reset — made or called off here, never sent by itself.

import { act } from "@/app/act";
import type { LimitWindow, PaneLimit } from "@/features/roster/api";
import { ago } from "@/lib/time";
import { Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { bookContinue, cancelContinue } from "./api";
import { resetForHit, resetTime } from "./limits";

export function LimitNotice({ machineId, paneId, agent, limit, windows, viewOnly }: {
  machineId: string;
  paneId: string;
  agent: string;
  limit?: PaneLimit;
  windows: LimitWindow[]; // the account's windows, the freshest reading of each
  viewOnly: boolean;
}) {
  if (!limit?.hit_at && !limit?.continue_at) return null;
  const now = Date.now();
  const reset = resetForHit(windows, agent);
  const booked = limit.continue_at;
  const action = viewOnly ? undefined : booked ? (
    <button type="button" className="linkish" onClick={() => void act(cancelContinue(machineId, paneId))}>
      {t("Call it off")}
    </button>
  ) : reset ? (
    <button type="button" className="linkish" onClick={() => void act(bookContinue(machineId, paneId, reset))}>
      {t("Continue after the reset")}
    </button>
  ) : undefined;
  return (
    <Notice tone="warn" icon="clock" className="chat-notice" action={action}>
      {limit.hit_at ? t("The agent stopped on its plan limit {ago}.", { ago: ago(limit.hit_at) }) : null}{" "}
      {booked
        ? t("At {time} remuda says “continue” to it, as you booked.", { time: resetTime(booked, now) })
        : reset
          ? t("The limit resets {time}.", { time: resetTime(reset, now) })
          : t("When the limit resets is not known here: Codex's come from its files, Claude Code's through its status line.")}
    </Notice>
  );
}
