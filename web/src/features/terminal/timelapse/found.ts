// What the search over the recorded screens comes to on the page: every place the text stood laid out as steps through
// the recordings shown, the step the player stands on, the next one either way, and the words for a hit and for what
// went unread. Pure.

import { t, tn } from "@/lib/i18n";
import { spent } from "@/lib/time";
import type { Appearance, ScreenHit, ScreenResult } from "./api";

// Step is one stretch the text stood on a screen: the recording, its first frame and moment, the row it stood in
export type Step = { recording: string; frame: number; at: number; line: number };
// TimelapseFound is a hit the player opens at, from the search panel: the query and its step
export type TimelapseFound = Step & { query: string };

const stepOf = (recording: string, seen: Appearance): Step => ({ recording, frame: seen.frame, at: seen.at, line: seen.line });

// stepsOf is every appearance of the hits in the recordings shown, in the order they are shown — the pane's newest
// first — and by time within each; a recording not shown (a hidden folder's) has no step
export function stepsOf(hits: ScreenHit[], shown: string[]): Step[] {
  const byId = new Map(hits.map((hit) => [hit.recording, hit]));
  const out: Step[] = [];
  for (const id of shown) {
    const hit = byId.get(id);
    if (!hit) continue;
    for (const seen of [hit.first, ...(hit.again ?? [])].sort((a, b) => a.frame - b.frame)) out.push(stepOf(id, seen));
  }
  return out;
}

// firstStep is a hit's first appearance as a step: where the search panel opens the player
export const firstStep = (hit: ScreenHit): Step => stepOf(hit.recording, hit.first);

// stepAt is the step the player stands on, by its recording and frame; -1 for none
export const stepAt = (steps: Step[], at: Pick<Step, "recording" | "frame"> | null): number => (at ? steps.findIndex((s) => s.recording === at.recording && s.frame === at.frame) : -1);

// stepFrom is the step after (1) or before (-1) the one at `at`, round and round; from none, the first or the last
export function stepFrom(count: number, at: number, by: 1 | -1): number {
  if (count <= 0) return -1;
  if (at < 0) return by > 0 ? 0 : count - 1;
  return (at + by + count) % count;
}

// stepWords says "3 of 12", how many there are while none is picked, "0 of 0" for none
export function stepWords(at: number, total: number): string {
  if (total <= 0) return t("0 of 0");
  if (at < 0) return tn(total, "{n} match", "{n} matches");
  return t("{at} of {total}", { at: at + 1, total });
}

// momentOf is when a hit's text first stood on the screen
export const momentOf = (hit: Pick<ScreenHit, "start" | "first">): Date => new Date(Date.parse(hit.start) + hit.first.at);

// stayWords says how long the text first stood there, and how often it came back
export function stayWords(hit: Pick<ScreenHit, "first" | "times">): string {
  const stayed = t("on screen for {time}", { time: spent(hit.first.for / 1000) });
  return hit.times > 1 ? `${stayed} · ${tn(hit.times, "appeared {n} time", "appeared {n} times")}` : stayed;
}

// unreadWords says what a search left out: recordings past MaxHits, or the older ones its budget or its time did not
// reach; "" when it read every one
export function unreadWords(result: Pick<ScreenResult, "more" | "unread" | "cut" | "hits">): string {
  if (result.more) return t("Only the newest {n} recordings that show it — narrow it down with a folder or a period.", { n: result.hits.length });
  if (!result.cut || result.unread <= 0) return "";
  return tn(result.unread, "{n} older recording was not searched — narrow it down with a folder or a period.", "{n} older recordings were not searched — narrow it down with a folder or a period.");
}

// excerptRows is a hit's screen excerpt as rows, the matching one marked, a cut on the left shown as an ellipsis
export function excerptRows(hit: Pick<ScreenHit, "before" | "row" | "after" | "col">): { text: string; match: boolean }[] {
  const lead = hit.col > 0 ? "…" : "";
  return [...hit.before.map((text) => ({ text, match: false })), { text: hit.row, match: true }, ...hit.after.map((text) => ({ text, match: false }))].map((row) => ({ ...row, text: row.text ? lead + row.text : "" }));
}
