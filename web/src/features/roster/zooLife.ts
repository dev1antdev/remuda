// What happens between the zoo's creatures while the page watches: a finish has the resting ones clap, a merged pull
// request sets off fireworks over the zoo, a compaction a puff, a question a chirp; the eyes follow the pointer; the
// asking ones hold up what they ask; a line along the foot tells the day. Only changes seen on this page count — the
// first reading seeds, so a reload replays nothing. The pure part; app/zoo plays it.

import type { Question } from "@/features/chat/api";
import { questionLines } from "@/features/chat/question";
import type { Standup } from "@/features/standup/api";
import { usd } from "@/features/usage/format";
import { t, tn } from "@/lib/i18n";
import type { PaneUsage } from "./api";
import type { Display } from "./radar";
import { BUBBLE_MAX, cutAt, type Pose } from "./zoo";
import { compacted } from "./zooBody";
import type { Species } from "./zooWear";

// Seen is what the page noted of a creature at one reading
export type Seen = { display: Display; pr: string; usage?: Pick<PaneUsage, "context" | "window"> };

export type Happenings = { finished: string[]; asked: string[]; merged: string[]; compacted: string[] };

// happenings are the moves between two readings, by creature key; a creature new at the second reading moved nowhere
export function happenings(before: Map<string, Seen>, after: Map<string, Seen>): Happenings {
  const out: Happenings = { finished: [], asked: [], merged: [], compacted: [] };
  for (const [key, now] of after) {
    const was = before.get(key);
    if (!was) continue;
    if (was.display === "working" && now.display === "done") out.finished.push(key);
    if (was.display !== "blocked" && now.display === "blocked") out.asked.push(key);
    if (was.pr !== "MERGED" && now.pr === "MERGED") out.merged.push(key);
    if (compacted(was.usage, now.usage)) out.compacted.push(key);
  }
  return out;
}

// joinsApplause says whether a creature in a pose claps for another's finish: the resting and the puzzled wake for it,
// the busy keep typing, the asking and the worried have their own trouble
export const joinsApplause = (pose: Pose): boolean => pose === "asleep" || pose === "puzzled";

// CHEER_MS, PUFF_MS and FIREWORKS_MS are how long the applause, a compaction's puff and the fireworks last
export const CHEER_MS = 2600;
export const PUFF_MS = 2400;
export const FIREWORKS_MS = 5200;

// GAZE_MAX is how far a pupil moves off the eye's centre, in the creature's own units
export const GAZE_MAX = 2.2;

// gaze is where a creature's pupils look: towards the pointer, the full way once it is reach pixels off or more
export function gaze(center: { x: number; y: number }, pointer: { x: number; y: number }, reach = 360): { x: number; y: number } {
  const dx = pointer.x - center.x;
  const dy = pointer.y - center.y;
  const distance = Math.hypot(dx, dy);
  if (distance < 1) return { x: 0, y: 0 };
  const pull = (GAZE_MAX * Math.min(1, distance / reach)) / distance;
  return { x: Math.round(dx * pull * 100) / 100, y: Math.round(dy * pull * 100) / 100 };
}

// questionBubble is what an asking creature holds up: the dialog's head and the first line of its detail ("Bash command ·
// rm -rf build"), else the line that asks
export function questionBubble(question: Question | null | undefined): string {
  if (!question?.text) return "";
  const { head, detail, ask } = questionLines(question.text);
  const first = detail.split("\n")[0] ?? "";
  const text = head ? (first ? `${head} · ${first}` : head) : ask;
  return cutAt(text, BUBBLE_MAX);
}

// tickerFacts are the day along the zoo's foot: answers, waits, spend, merges and who worked longest, each when it says
// something; nameOf gives the creature's name as the zoo shows it, or null for one it does not show
export function tickerFacts(standup: Standup | null, merged: number, nameOf: (machine: string, pane: string) => string | null): string[] {
  const facts: string[] = [];
  const items = standup?.items ?? [];
  const answers = items.reduce((sum, item) => sum + (item.recap.answers || 0), 0);
  if (answers) facts.push(tn(answers, "Today: {n} answer", "Today: {n} answers"));
  const waiting = items.filter((item) => item.state === "blocked").length;
  if (waiting) facts.push(tn(waiting, "{n} waits for you", "{n} wait for you"));
  const spent = items.reduce((sum, item) => sum + (item.recap.cost || 0), 0);
  if (spent >= 0.01) facts.push(t("{cost} spent", { cost: usd(spent) }));
  if (merged) facts.push(tn(merged, "{n} pull request merged", "{n} pull requests merged"));
  let longest: { name: string; took: number } | null = null;
  for (const item of items) {
    const name = nameOf(item.machine, item.pane);
    if (name && item.recap.took > (longest?.took ?? 0)) longest = { name, took: item.recap.took };
  }
  if (longest && longest.took >= 60) facts.push(t("worked longest: {name}, {time}", { name: longest.name, time: workedWords(longest.took) }));
  return facts;
}

// workedWords is a stretch of work: "42 min", "2 h 10 min"
export function workedWords(seconds: number): string {
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return t("{n} min", { n: minutes });
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest ? t("{h} h {m} min", { h: hours, m: rest }) : t("{h} h", { h: hours });
}

export type Voice = { wave: OscillatorType; base: number };

// voiceOf is a species' voice: its waveform and its pitch in Hz
export function voiceOf(species: Species): Voice {
  switch (species) {
    case "codex":
      return { wave: "square", base: 392 };
    case "gemini":
      return { wave: "sine", base: 784 };
    case "kimi":
      return { wave: "triangle", base: 523 };
    case "other":
      return { wave: "sawtooth", base: 330 };
    default:
      return { wave: "sine", base: 659 };
  }
}

export type Call = "ask" | "finish" | "merge";

// TUNES are each call's notes, as steps over the species' pitch in semitones
export const TUNES: Record<Call, number[]> = { ask: [0, 5], finish: [0, 4, 7], merge: [0, 4, 7, 12] };
