// The review queue: every workspace with uncommitted work, the ones with changes since the reviewer looked first,
// in the roster's order otherwise. Pure over the roster, tested; ReviewQueue.tsx draws it.

import type { Machine, Pane, Session } from "@/features/roster/api";

export type QueueItem = {
  key: string; // machine|workspace
  machine: Machine;
  session: Session;
  dir: string;
  pane: Pane | null; // the workspace's first agent pane, else its first pane: whose Changes lens this is
  fresh: number; // files moved since the last look
  changed: number; // files changed against HEAD, untracked ones too
};

export function reviewQueue(machines: Machine[]): QueueItem[] {
  const items: QueueItem[] = [];
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const session of machine.sessions) {
      if (!session.dir || !session.review || session.review.changed === 0) continue;
      const panes = session.tabs.flatMap((tab) => tab.panes);
      items.push({ key: `${machine.id}|${session.id}`, machine, session, dir: session.dir, pane: panes.find((pane) => pane.agent) ?? panes[0] ?? null, fresh: session.review.fresh, changed: session.review.changed });
    }
  }
  return items.sort((a, b) => Number(b.fresh > 0) - Number(a.fresh > 0));
}
