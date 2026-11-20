// The chats a reboot stopped on a machine, as the roster lists them, and herdr's agents that can carry on there once it
// moved to remuda-core: the words for the ⓘ and the toast, and the key of one episode, so a toast dismissed stays
// dismissed until the list changes.

import { t, tn } from "@/lib/i18n";
import { VENDOR_NAME } from "@/ui/brand";
import type { StoppedChat } from "./api";

// episodeKey names one set of stopped chats on a machine; a chat resumed or forgotten makes a new episode
export const episodeKey = (machine: string, stopped: StoppedChat[]): string =>
  `${machine}:${stopped
    .map((s) => s.pane)
    .sort()
    .join(",")}`;

// stoppedWords is the count as a sentence: "3 chats stopped with the reboot"
export const stoppedWords = (n: number): string => tn(n, "{n} chat stopped with the reboot", "{n} chats stopped with the reboot");

// stoppedNames lists what stopped: "Claude Code in api, Codex in web"
export const stoppedNames = (stopped: StoppedChat[]): string => stopped.map((s) => t("{agent} in {title}", { agent: VENDOR_NAME[s.agent] ?? s.agent, title: s.title || s.pane })).join(", ");

// fromHerdr is an agent that ran in herdr before the machine moved to remuda-core
export const fromHerdr = (chat: StoppedChat): boolean => chat.from === "herdr";

// herdrWords is the count of herdr's agents as a sentence: "2 agents from herdr on GOG can carry on"
export const herdrWords = (n: number, machine: string): string => tn(n, "{n} agent from herdr on {machine} can carry on", "{n} agents from herdr on {machine} can carry on", { machine });

// concernWords is the ⓘ's line: what a reboot stopped and what herdr left, each with its names
export function concernWords(stopped: StoppedChat[], machine: string): string {
  const herdr = stopped.filter(fromHerdr);
  const rebooted = stopped.filter((chat) => !fromHerdr(chat));
  const parts = [rebooted.length ? `${stoppedWords(rebooted.length)}: ${stoppedNames(rebooted)}` : "", herdr.length ? `${herdrWords(herdr.length, machine)}: ${stoppedNames(herdr)}` : ""];
  return parts.filter(Boolean).join(" · ");
}

// toastWords is the toast's line: "GOG: 1 chat stopped with the reboot · 2 agents from herdr on GOG can carry on"
export function toastWords(stopped: StoppedChat[], machine: string): string {
  const herdr = stopped.filter(fromHerdr).length;
  const rebooted = stopped.length - herdr;
  return [rebooted ? `${machine}: ${stoppedWords(rebooted)}` : "", herdr ? herdrWords(herdr, machine) : ""].filter(Boolean).join(" · ");
}

// MAX_SEEN is how many dismissed episodes the device keeps
export const MAX_SEEN = 8;

// remember adds a dismissed episode to the list, the oldest dropped past MAX_SEEN
export const remember = (seen: string[], key: string): string[] => [...seen.filter((k) => k !== key), key].slice(-MAX_SEEN);
