// One chat started again on its session, by hand: a confirm, the job, a toast while it runs and one when it ends.

import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { VENDOR_NAME } from "@/ui/brand";
import { fetchJob, restartPane } from "./api";
import { t } from "@/lib/i18n";

const POLL = 1000;

export const agentTitle = (agent: string): string => VENDOR_NAME[agent] ?? agent;

// askRestart is the confirm before the restart; a yes starts it.
export function askRestart(machineId: string, paneId: string, agent: string): void {
  const name = agentTitle(agent);
  actions.ask(`Exits ${name} and starts it again on the same conversation. A question open is refused; a message half-typed in its box is lost.`, t("Restart on its session"), () => void restartOnSession(machineId, paneId, name));
}

async function restartOnSession(machineId: string, paneId: string, name: string): Promise<void> {
  try {
    const { job } = await restartPane(machineId, paneId);
    actions.notify(`Restarting ${name}…`);
    const ended = await untilDone(job);
    if (ended.ok) actions.notify(`${name} is back on its session`);
    else actions.notify(ended.error || `${name} did not come back`, { error: true });
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}

async function untilDone(id: string) {
  for (;;) {
    const job = await fetchJob(id);
    if (job.done) return job;
    await new Promise((resolve) => setTimeout(resolve, POLL));
  }
}
