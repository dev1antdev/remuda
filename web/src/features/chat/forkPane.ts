// forkPane forks a pane's conversation into a new tab of its workspace — the session shown, else the live one — then
// selects the new pane and says how it went.

import { act } from "@/app/act";
import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { forkConversation } from "./api";
import { forkedWords } from "./fork";

export async function forkPane(machineId: string, paneId: string, agent: string, session?: string | null): Promise<void> {
  try {
    const made = await forkConversation(machineId, paneId, session ?? undefined);
    await act(Promise.resolve());
    actions.selectPane(machineId, made.pane);
    const words = forkedWords(agent, made.agent_error);
    actions.notify(words.text, { error: words.error });
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}
