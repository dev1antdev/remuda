// The chat lens's tools: Find stays in the tab strip's slot as the one control; the timeline, the recap, the export,
// the pins and the handover to another agent are rows of the View menu (useChatViewItems), for the pane's conversation
// and — but for the handover, which needs a pane — for a transcript on its own.

import { actions, app } from "@/app/store";
import { usePref } from "@/lib/prefs";
import { errMsg, plural } from "@/lib/text";
import type { MenuItem } from "@/ui";
import { exportConversation } from "./export";
import { FindTool } from "./FindBar";
import { pinKey, type Pin } from "./pins";
import { useTimelineShown } from "./TimelineBar";

export type ChatPlace = { machineId: string; paneId: string; session: string | null; dir: string; agent: string; workspace: string; machine: string };

export function ChatTools() {
  return <FindTool />;
}

// useChatViewItems is the chat's part of the View menu; a shell pane has no conversation, so its rows are disabled
export function useChatViewItems(place: ChatPlace): MenuItem[] {
  const [timeline, setTimeline] = useTimelineShown();
  const { machineId, paneId, session, dir, agent, workspace, machine } = place;
  const chatLive = app.use((s) => s.chatLive);
  const [pins] = usePref<Pin[]>(pinKey(machineId, dir), []);
  const title = machineId !== "local" && machine ? `${workspace} on ${machine}` : workspace;
  const save = () => void exportConversation(machineId, paneId, session, dir, { agent, workspace, machine }).catch((e) => actions.notify(errMsg(e), { error: true }));
  return [
    { label: "Timeline", icon: "clock", hint: "every prompt and answer in time; the first error, the longest call, loops and questions marked", checked: timeline, onSelect: () => setTimeline(!timeline) },
    { sep: true },
    { label: "Recap this conversation…", icon: "sun", hint: "prompts, answers, tools, files, cost", disabled: !agent, onSelect: () => actions.openDialog({ kind: "recap", machineId, paneId, session, dir, title }) },
    { label: "Export as Markdown", icon: "download", hint: "the conversation shown, as a file", disabled: !agent, onSelect: save },
    { sep: true },
    { label: "Continue with another agent…", icon: "arrow-right", hint: "a new tab here, the gist of this conversation as its first prompt", disabled: !agent || !paneId, onSelect: () => actions.openDialog({ kind: "handover", machineId, paneId, session, agent, workspace: title }) },
    { sep: true },
    { label: "Pins…", icon: "pin", hint: pins.length ? plural(pins.length, "pinned turn") : "the turns you pin, each a jump back to it", disabled: !agent || !paneId, onSelect: () => actions.openDialog({ kind: "pins", machineId, paneId, dir, shown: session ?? chatLive, title }) },
  ];
}
