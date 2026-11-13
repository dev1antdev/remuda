// A workspace put to sleep and woken from its menu or the palette: the confirm when a sleep would interrupt someone,
// the job followed, a toast while it runs and one when it ends. Exported for the other places that offer Sleep and
// Wake beside a workspace.

import { actions } from "@/app/store";
import { untilDone } from "@/features/updates/restartPane";
import type { Session } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { errMsg } from "@/lib/text";
import { sleepWorkspace, wakeWorkspace, type Busy } from "./api";
import { busyOf, sleepAsk, sleptWords, wokeWords } from "./sleep";

// askSleep puts the workspace to sleep, asking first when its agents work or ask: Orca's confirm with Sleep anyway
export function askSleep(machineId: string, session: Session): void {
  const busy = busyOf(session);
  if (busy.length) confirmSleep(machineId, session, busy);
  else void sleep(machineId, session, false);
}

function confirmSleep(machineId: string, session: Session, busy: Busy[]) {
  actions.ask(sleepAsk(session.name, busy), t("Sleep anyway"), () => void sleep(machineId, session, true));
}

async function sleep(machineId: string, session: Session, anyway: boolean): Promise<void> {
  try {
    const started = await sleepWorkspace(machineId, session.id, anyway);
    if (started.busy) {
      confirmSleep(machineId, session, started.busy); // the roster was behind: someone started working meanwhile
      return;
    }
    actions.notify(t("Putting {name} to sleep…", { name: session.name }));
    const ended = await untilDone(started.job);
    if (ended.ok) actions.notify(sleptWords(session.name, started.agents));
    else actions.notify(ended.error || t("{name} did not go to sleep", { name: session.name }), { error: true });
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}

// wake starts the agents a sleep stopped again on their sessions; a partial wake says what stays asleep and why
export async function wake(machineId: string, session: Session): Promise<void> {
  try {
    const { job, agents } = await wakeWorkspace(machineId, session.id);
    actions.notify(t("Waking {name}…", { name: session.name }));
    const ended = await untilDone(job);
    if (ended.ok) actions.notify(wokeWords(session.name, agents));
    else actions.notify(ended.error || t("{name} is still partly asleep", { name: session.name }), { error: true });
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}
