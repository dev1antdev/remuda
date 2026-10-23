// What this device says aloud about the agents, from one roster to the next: a finish with what was answered, a question
// opened, a watchdog note — each once, by a key per episode the hook remembers, so a line is said on the change and never
// again on a reload or a poll.

import type { Machine } from "@/features/roster/api";

export type VoicePrefs = { on: boolean; voice?: string }; // voice: the browser voice's name; none for the default
export const DEFAULT_VOICE: VoicePrefs = { on: false };

export type Line = { key: string; text: string; lang: string };

// linesToSay walks every agent pane on a connected machine: the lines whose key was not heard yet, and every key there is now
export function linesToSay(machines: Machine[], heard: Set<string>): { lines: Line[]; keys: Set<string> } {
  const lines: Line[] = [];
  const keys = new Set<string>();
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const session of machine.sessions)
      for (const tab of session.tabs)
        for (const pane of tab.panes) {
          const radar = pane.radar;
          if (!pane.agent || !radar) continue;
          const id = `${machine.id}|${pane.id}`;
          const label = machine.id === "local" ? session.name : `${session.name} on ${machine.label}`;
          let key = "";
          let text = "";
          if (radar.alert) {
            key = `${id}|alert|${radar.alert}`;
            text = `${label}: ${radar.alert}`;
          } else if (radar.state === "blocked") {
            key = `${id}|asks|${radar.since ?? ""}`;
            text = `${label} is waiting for you`;
          } else if (radar.state === "done" && pane.said) {
            key = `${id}|done|${radar.since ?? ""}`;
            text = `${label}: ${pane.said}`;
          } else continue;
          keys.add(key);
          if (!heard.has(key)) lines.push({ key, text, lang: langOf(text) });
        }
  }
  return { lines, keys };
}

// langOf is the voice's language for a line: Russian for Cyrillic text, else the browser's own choice ("")
export function langOf(text: string): string {
  return /[Ѐ-ӿ]/.test(text) ? "ru" : "";
}
