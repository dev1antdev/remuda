// useDayScript is the standup as the voice reads it on this page: the report, the reviews waiting and the plan limits
// near their end off the roster, and the screen-sharing mode's neutral names for a hidden folder or machine — its files
// and its words left out, every other agent's words with the hidden names swapped.

import { useCallback } from "react";
import type { Machine, Session } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { folderOf } from "@/features/roster/workspaces";
import { reviewQueue } from "@/features/review/queue";
import { accountWindows } from "@/features/usage/limits";
import { t } from "@/lib/i18n";
import type { Part } from "@/lib/speech";
import type { Item, Standup } from "./api";
import { spokenName } from "./speakable";
import { dayScript, type Who } from "./spoken";
import type { Period } from "./standup";

// useDayScript gives the function that writes the script now, from the report and the roster as they stand
export function useDayScript(data: Standup | null, period: Period): () => Part[] {
  const { machines } = useRoster();
  const share = useSharing();
  return useCallback(() => {
    if (!data) return [];
    const places = new Map<string, { machine: Machine; session: Session }>();
    for (const machine of machines) for (const session of machine.sessions) for (const tab of session.tabs) for (const pane of tab.panes) places.set(`${machine.id}|${pane.id}`, { machine, session });
    const whoOf = (item: Item): Who => {
      const at = places.get(`${item.machine}|${item.pane}`);
      const hidden = at ? share.sessionHidden(item.machine, at.session) : share.machineHidden(item.machine);
      const workspace = hidden && at ? share.folderName(item.machine, folderOf(at.session), item.workspace) : item.workspace;
      const machine = item.machine === "local" ? "" : at ? share.machineLabel(at.machine) : item.label;
      return { name: machine ? t("{workspace} on {machine}", { workspace: spokenName(workspace), machine: spokenName(machine) }) : spokenName(workspace), hidden };
    };
    const reviews = reviewQueue(machines)
      .filter((review) => review.ready || review.fresh > 0)
      .map((review) => {
        const hidden = share.sessionHidden(review.machine.id, review.session);
        return { name: spokenName(hidden ? share.folderName(review.machine.id, folderOf(review.session), review.session.name) : review.session.name), ready: review.ready };
      });
    const now = Date.now();
    return dayScript({ items: data.items, period, now, whoOf, quote: share.redact, reviews, limits: accountWindows(machines, now) });
  }, [data, period, machines, share]);
}
