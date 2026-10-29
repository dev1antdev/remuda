// The chats a reboot stopped on a machine, started again on their sessions: the job, a toast while it runs and
// one when it ends; the log sits in the machine's Updates under its recent jobs.

import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { t, tn } from "@/lib/i18n";
import { recoverChats } from "./api";
import { untilDone } from "./restartPane";

export async function resumeStopped(machineId: string, count: number): Promise<void> {
  try {
    const { job } = await recoverChats(machineId);
    actions.notify(tn(count, "Resuming {n} chat…", "Resuming {n} chats…"));
    const ended = await untilDone(job);
    if (ended.ok) actions.notify(tn(count, "The chat is back on its session", "The chats are back on their sessions"));
    else actions.notify(ended.error || t("Not every chat came back"), { error: true });
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}
