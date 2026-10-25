// The chat lens's tools: Find stays in the tab strip's slot as the one control; the timeline, the recap and the export
// are rows of the View menu (useChatViewItems), for the pane's conversation and for a transcript on its own.

import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import type { MenuItem } from "@/ui";
import { exportConversation } from "./export";
import { FindTool } from "./FindBar";
import { useTimelineShown } from "./TimelineBar";

export type ChatPlace = { machineId: string; paneId: string; session: string | null; dir: string; agent: string; workspace: string; machine: string };

export function ChatTools() {
  return <FindTool />;
}

// useChatViewItems is the chat's part of the View menu; a shell pane has no conversation, so its rows are disabled
export function useChatViewItems(place: ChatPlace): MenuItem[] {
  const [timeline, setTimeline] = useTimelineShown();
  const { machineId, paneId, session, dir, agent, workspace, machine } = place;
  const title = machineId !== "local" && machine ? `${workspace} on ${machine}` : workspace;
  const save = () => void exportConversation(machineId, paneId, session, dir, { agent, workspace, machine }).catch((e) => actions.notify(errMsg(e), { error: true }));
  return [
    { label: "Timeline", icon: "clock", hint: "every prompt and answer in time; the first error, the longest call, loops and questions marked", checked: timeline, onSelect: () => setTimeline(!timeline) },
    { sep: true },
    { label: "Recap this conversation…", icon: "sun", hint: "prompts, answers, tools, files, cost", disabled: !agent, onSelect: () => actions.openDialog({ kind: "recap", machineId, paneId, session, dir, title }) },
    { label: "Export as Markdown", icon: "download", hint: "the conversation shown, as a file", disabled: !agent, onSelect: save },
  ];
}
