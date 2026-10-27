// Pinned turns: bookmarks on a pane's conversation, kept on the page per workspace folder — so a pin outlives a
// compaction's new session file and the list shows every session of the folder — and shared between devices through the
// settings sync. The pure part; Chat, TurnView and PinsDialog use it.

import { ago } from "@/lib/time";
import type { Turn } from "./api";
import { firstLine } from "./said";
import { t } from "@/lib/i18n";

export type Pin = {
  session: string; // the session file the turn is in
  turn: string; // the turn's first record uuid, what a jump and a turn link name
  line: string; // the turn's first line, for the list
  at: string; // when it was pinned, ISO
  kind: "answer" | "prompt";
};

export const MAX_PINS = 200;

// pinKey is the pref every pin of a folder's conversations lives under
export function pinKey(machineId: string, dir: string): string {
  return `pins:${machineId}|${dir}`;
}

export function isPinned(pins: Pin[], turn: string): boolean {
  return pins.some((p) => p.turn === turn);
}

// togglePin takes a pinned turn out, else puts the pin first; the oldest past MAX_PINS go
export function togglePin(pins: Pin[], pin: Pin): Pin[] {
  if (isPinned(pins, pin.turn)) return pins.filter((p) => p.turn !== pin.turn);
  return [pin, ...pins].slice(0, MAX_PINS);
}

// pinOf is the pin for a turn of the session shown; nothing for a turn without a record to name it by
export function pinOf(turn: Turn, session: string, now: Date = new Date()): Pin | null {
  const id = turn.ids?.[0];
  if (!id) return null;
  return { session, turn: id, line: pinLine(turn), at: now.toISOString(), kind: turn.role === "user" ? "prompt" : "answer" };
}

// pinLine is what the list shows for a turn: the first line of the text it ended with, else of any text in it,
// else the tools it called
export function pinLine(turn: Turn): string {
  let ending: string[] = [];
  const all: string[] = [];
  const tools: string[] = [];
  for (const part of turn.parts) {
    if (part.type === "tool") {
      ending = [];
      if (part.tool_name && !tools.includes(part.tool_name)) tools.push(part.tool_name);
    } else if (part.type === "text" && part.text?.trim()) {
      ending.push(part.text.trim());
      all.push(part.text.trim());
    }
  }
  return firstLine(ending.join("\n\n")) || firstLine(all.join("\n\n")) || (tools.length ? `(${tools.join(", ")})` : "(nothing said)");
}

// shortSession is how a session is told apart in a list: its first eight characters
export function shortSession(id: string): string {
  return id.slice(0, 8);
}

// pinWords says when the pin was made and, for a turn in another session than the one shown, which one
export function pinWords(pin: Pin, shown: string | null, now = Date.now()): string {
  const when = ago(pin.at, now);
  return pin.session && pin.session !== shown ? t("{when} · session {id}", { when, id: shortSession(pin.session) }) : when;
}
