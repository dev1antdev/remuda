// Fetch, pull and push from the Git lens, through the Repositories place's own calls and words: one at a time, its
// outcome a toast, a push the secret check holds kept for Push anyway, and the history read again after each.

import { useState } from "react";
import { actions } from "@/app/store";
import { outcomeWords } from "@/features/repos/outcome";
import { syncRepo, type Outcome, type SyncAction } from "@/features/repos/syncApi";
import { basename, errMsg } from "@/lib/text";

export function useSync(machineId: string, dir: string, after: () => void) {
  const [running, setRunning] = useState<SyncAction | "">("");
  const [held, setHeld] = useState<Outcome | null>(null);
  const name = basename(dir) || dir;
  // pressed is the held push the person said Push anyway to; noVerify a push without the repository's pre-push hook
  const run = async (action: SyncAction, pressed?: Outcome, noVerify = false) => {
    if (running) return;
    setRunning(action);
    try {
      const outcome = await syncRepo(action, machineId, dir, pressed, noVerify);
      if (outcome.state === "held") setHeld(outcome);
      else {
        const said = outcomeWords(outcome, name);
        actions.notify(said.text, { error: said.error });
      }
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
    } finally {
      setRunning("");
      after();
    }
  };
  return { running, held, name, run, dropHeld: () => setHeld(null) };
}
