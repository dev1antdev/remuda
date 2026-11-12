// The chat lens's tools: Find stays in the tab strip's slot as the one control; the timeline, the recap, the export,
// the timelapse of the pane's screen, the pins and the handover to another agent are rows of the View menu
// (useChatViewItems), for the pane's conversation and — but for the timelapse and the handover, which need a pane — for
// a transcript on its own.

import { actions, app } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { errMsg, plural } from "@/lib/text";
import type { MenuItem } from "@/ui";
import { exportConversation } from "./export";
import { canFork, forkLabel } from "./fork";
import { forkPane } from "./forkPane";
import { FindTool } from "./FindBar";
import { pinKey, type Pin } from "./pins";
import { useTimelineShown } from "./TimelineBar";
import { timelapseRows } from "@/features/terminal/timelapse/row";
import { t } from "@/lib/i18n";

export type ChatPlace = { machineId: string; paneId: string; session: string | null; dir: string; agent: string; workspace: string; machine: string; model?: string };

export function ChatTools() {
  return <FindTool />;
}

// useChatViewItems is the chat's part of the View menu; a shell pane has no conversation, so its rows are disabled
export function useChatViewItems(place: ChatPlace): MenuItem[] {
  const [timeline, setTimeline] = useTimelineShown();
  const { machineId, paneId, session, dir, agent, workspace, machine, model } = place;
  const chatLive = app.use((s) => s.chatLive);
  const { viewOnly } = useRoster();
  const [pins] = usePref<Pin[]>(pinKey(machineId, dir), []);
  const title = machineId !== "local" && machine ? t("{workspace} on {machine}", { workspace, machine }) : workspace;
  const save = () => void exportConversation(machineId, paneId, session, dir, { agent, workspace, machine }).catch((e) => actions.notify(errMsg(e), { error: true }));
  return [
    { label: t("Timeline"), icon: "clock", hint: t("every prompt and answer in time; the first error, the longest call, loops and questions marked"), checked: timeline, onSelect: () => setTimeline(!timeline) },
    { sep: true },
    { label: t("Recap this conversation…"), icon: "sun", hint: t("prompts, answers, tools, files, cost"), disabled: !agent, onSelect: () => actions.openDialog({ kind: "recap", machineId, paneId, session, dir, title }) },
    { label: t("Export as Markdown"), icon: "download", hint: t("the conversation shown, as a file"), disabled: !agent, onSelect: save },
    ...(paneId ? timelapseRows(place).slice(1) : []),
    // a viewer link may read but not start anything: no handover, no fork, no model switch
    ...(viewOnly
      ? []
      : [
          { sep: true } as const,
          { label: t("Continue with another agent…"), icon: "arrow-right" as const, hint: t("a new tab here, the gist of this conversation as its first prompt"), disabled: !agent || !paneId, onSelect: () => actions.openDialog({ kind: "handover", machineId, paneId, session, agent, workspace: title }) },
          ...(paneId && canFork(agent) ? [{ label: forkLabel(agent), icon: "columns" as const, hint: t("the same history in a new tab, a second path; this one goes on as it is"), onSelect: () => void forkPane(machineId, paneId, agent, session) }] : []),
        ]),
    { sep: true },
    { label: t("Pins…"), icon: "pin", hint: pins.length ? plural(pins.length, "pinned turn") : t("the turns you pin, each a jump back to it"), disabled: !agent || !paneId, onSelect: () => actions.openDialog({ kind: "pins", machineId, paneId, dir, shown: session ?? chatLive, title }) },
    ...(viewOnly ? [] : [{ sep: true } as const, { label: t("Switch the model…"), icon: "sparkles" as const, hint: t("Opus, Sonnet, Haiku or an id, through Claude Code's own /model"), disabled: agent !== "claude" || !paneId, onSelect: () => actions.openDialog({ kind: "model", machineId, paneId, current: model }) }]),
  ];
}
