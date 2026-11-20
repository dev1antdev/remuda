// The chats a reboot stopped on a machine, started again on their sessions: the job followed with a toast while it runs
// and one when it ends — the log sits in the machine's Updates under its recent jobs — and herdr's server stopped at the
// person's word, so the agents it ran there can carry on in remuda-core.

import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { t, tn } from "@/lib/i18n";
import { stopHerdr } from "./api";
import { untilDone } from "./restartPane";

// followResume follows a resume's job; count is how many chats it resumes
export async function followResume(job: string, count: number): Promise<void> {
  actions.notify(tn(count, "Resuming {n} chat…", "Resuming {n} chats…"));
  try {
    const ended = await untilDone(job);
    if (ended.ok) actions.notify(tn(count, "The chat is back on its session", "The chats are back on their sessions"));
    else actions.notify(ended.error || t("Not every chat came back"), { error: true });
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}

// askStopHerdr stops herdr's server on the machine once the person said yes to what it ends; stopped hears it went
export function askStopHerdr(machineId: string, machine: string, stopped: () => void): void {
  const stop = async () => {
    try {
      await stopHerdr(machineId);
      actions.notify(t("herdr's server on {machine} is stopped", { machine }));
      stopped();
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
    }
  };
  actions.ask(t("Stop herdr's server on {machine}? All agents still running in herdr there stop; herdr stays installed.", { machine }), t("Stop herdr's server"), () => void stop());
}
