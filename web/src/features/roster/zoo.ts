// The zoo: every agent on the Wall as a little creature for the office TV. Its state reads from its pose and the sign over
// its head as well as from colour — busy at a laptop while it works, a hand up and a "?" while it asks, an amber "!" for a
// watchdog note, a dance for a finish nobody has looked at, asleep with a "z" when idle and dim when long idle, puzzled
// when its state is unknown. The pure part; app/Zoo.tsx draws the creatures.

import { t } from "@/lib/i18n";
import type { Pane } from "./api";
import type { Display } from "./radar";
import type { WallPrefs } from "./wall";

export type Pose = "busy" | "asking" | "worried" | "dancing" | "asleep" | "puzzled";
export type Sign = "?" | "!" | "z" | "…" | "";
export type Look = { pose: Pose; sign: Sign; dim: boolean };

// lookOf is a creature's pose and sign for an agent's state; a watchdog note wins over the rest
export function lookOf(display: Display, alert?: string): Look {
  if (alert) return { pose: "worried", sign: "!", dim: false };
  switch (display) {
    case "working":
      return { pose: "busy", sign: "", dim: false };
    case "blocked":
      return { pose: "asking", sign: "?", dim: false };
    case "done":
      return { pose: "dancing", sign: "", dim: false };
    case "unknown":
      return { pose: "puzzled", sign: "…", dim: false };
    case "idle_stale":
      return { pose: "asleep", sign: "z", dim: true };
    default:
      return { pose: "asleep", sign: "z", dim: false };
  }
}

// poseWord is the caption's word for a pose, before how long it has lasted
export function poseWord(pose: Pose): string {
  switch (pose) {
    case "busy":
      return t("working");
    case "asking":
      return t("asking you");
    case "worried":
      return t("needs a look");
    case "dancing":
      return t("finished");
    case "puzzled":
      return t("state unknown");
    default:
      return t("asleep");
  }
}

// CREATURE_SIZE is a creature's width in px per wall size; the featured one of office mode and the pop-out fill their box
export const CREATURE_SIZE: Record<WallPrefs["size"], number> = { s: 112, m: 156, l: 216 };

// cellMin is the narrowest grid cell for a size: the creature and room for its caption
export const cellMin = (size: WallPrefs["size"]) => CREATURE_SIZE[size] + 72;

// BUBBLE_MAX is how much of an answer the speech bubble holds
export const BUBBLE_MAX = 90;

// bubbleOf is the speech bubble over a creature: the first line of what the agent answered, once it finished or rests,
// cut at a word, never between the halves of a surrogate pair; nothing while it works, asks or carries a note
export function bubbleOf(pane: Pane, display: Display, alert?: string): string {
  if (alert || !pane.said) return "";
  if (display !== "done" && display !== "idle" && display !== "idle_fresh" && display !== "idle_stale") return "";
  const text = pane.said.trim();
  if (text.length <= BUBBLE_MAX) return text;
  const space = text.slice(0, BUBBLE_MAX).lastIndexOf(" ");
  let end = space > BUBBLE_MAX / 2 ? space : BUBBLE_MAX;
  const code = text.charCodeAt(end - 1);
  if (code >= 0xd800 && code <= 0xdbff) end -= 1;
  return `${text.slice(0, end).trimEnd()}…`;
}

// stateSince is when the state on show began: the start of the work or the question, else the last turn
export function stateSince(pane: Pane): number | undefined {
  const radar = pane.radar;
  if (!radar) return undefined;
  if ((radar.state === "working" || radar.state === "blocked") && radar.since) return radar.since;
  return radar.last_active || undefined;
}

export type ZooFace = { name: string; where: string; bubble: string; icon: boolean };

// zooFace is what a creature says of itself: its row label, its folder and machine, its bubble and its folder's picture;
// while screen sharing hides its folder, the agent's kind and the neutral names alone, no bubble and no picture
export function zooFace(face: { label: string; kind: string; folder: string; machine: string | null; bubble: string }, hidden: boolean): ZooFace {
  const where = `${face.folder}${face.machine ? ` · ${face.machine}` : ""}`;
  if (hidden) return { name: face.kind, where, bubble: "", icon: false };
  return { name: face.label, where, bubble: face.bubble, icon: true };
}
