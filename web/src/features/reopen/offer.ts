// After a close made through the page: find the agents it stopped on the server's closed list — the watcher notes them
// within a snapshot or two — and put up a toast that offers them back. Reopening goes through the same route as the
// dialog.

import { act } from "@/app/act";
import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { t } from "@/lib/i18n";
import { listClosed, reopenClosed } from "./api";
import { justClosed, offerWords } from "./closed";

const TRIES = 6;
const PAUSE_MS = 700;

// offerReopen looks for the agents the closed panes ran and offers them back; panes of shells alone offer nothing
export async function offerReopen(machineId: string, panes: string[]): Promise<void> {
  if (panes.length === 0) return;
  for (let i = 0; i < TRIES; i++) {
    await new Promise((resolve) => setTimeout(resolve, PAUSE_MS));
    const found = justClosed(await listClosed().catch(() => []), machineId, panes);
    if (found.length) {
      actions.setClosedOffer({ machineId, ids: found.map((c) => c.id), text: offerWords(found) });
      return;
    }
  }
}

// reopen starts closed agents again, one after another, and opens the first that came back; a refusal is a toast
export async function reopen(machineId: string, ids: string[]): Promise<void> {
  let first = "";
  for (const id of ids) {
    try {
      const back = await reopenClosed(machineId, id);
      first ||= back.pane;
    } catch (e) {
      actions.notify(t("Not reopened: {why}", { why: errMsg(e) }), { error: true });
    }
  }
  await act(Promise.resolve());
  if (first) actions.selectPane(machineId, first);
}
