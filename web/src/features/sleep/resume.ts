// "Resume after restart", after Orca's hibernate mod: the chats a reboot stopped, each with a tick, the command that
// brings it back, and the buttons' words. Pure; ResumeDialog.tsx draws it.

import { t } from "@/lib/i18n";
import { shellQuote } from "@/lib/text";
import { VENDOR_NAME } from "@/ui/brand";
import type { StoppedChat } from "@/features/roster/api";

// commandLine is the exact command a chat comes back with, in its folder; under the screen-sharing mode (masked) the
// folder is left out
export function commandLine(chat: StoppedChat, masked = false): string {
  const command = chat.command || `${chat.agent} --resume`;
  return chat.dir && !masked ? `cd ${shellQuote(chat.dir)} && ${command}` : command;
}

// chatName is the agent a row brings back
export const chatName = (chat: StoppedChat): string => VENDOR_NAME[chat.agent] ?? chat.agent;

// chatWhere is "workspace · machine"
export const chatWhere = (chat: StoppedChat, machine: string): string => [chat.title || chat.pane, machine].filter(Boolean).join(" · ");

// toggled is the ticks with one pane ticked or not
export const toggled = (picked: string[], pane: string): string[] => (picked.includes(pane) ? picked.filter((p) => p !== pane) : [...picked, pane]);

// stillPicked keeps the ticks of the chats still listed, so a chat resumed meanwhile leaves the count
export const stillPicked = (picked: string[], stopped: StoppedChat[]): string[] => picked.filter((pane) => stopped.some((chat) => chat.pane === pane));

// someLabel is the button for the ticked ones, shown while some but not every chat is ticked; null otherwise
export const someLabel = (picked: number, total: number): string | null => (picked > 0 && picked < total ? t("Resume {n}", { n: picked }) : null);
