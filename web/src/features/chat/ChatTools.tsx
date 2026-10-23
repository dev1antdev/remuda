// ChatTools sits in the tab strip's tools slot while the chat lens is on screen: the find bar and the export of the conversation shown.

import { actions, app } from "@/app/store";
import { errMsg } from "@/lib/text";
import { Icon } from "@/ui";
import { exportConversation } from "./export";
import { FindTool } from "./FindBar";

export function ChatTools({ machineId, paneId, agent, workspace, machine, dir }: { machineId: string; paneId: string; agent: string; workspace: string; machine: string; dir: string }) {
  const session = app.use((s) => s.chatSession);
  const save = () => void exportConversation(machineId, paneId, session, dir, { agent, workspace, machine }).catch((e) => actions.notify(errMsg(e), { error: true }));
  return (
    <>
      <FindTool />
      <button type="button" className="icon-btn sm" title="Export this conversation as a Markdown file (the session shown)" aria-label="Export this conversation as Markdown" disabled={!agent} onClick={save}>
        <Icon name="download" size={14} />
      </button>
    </>
  );
}
