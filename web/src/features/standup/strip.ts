// The day strip's pure part: the window it shows, the spans laid out on a bar of WIDTH units, the hour marks under it,
// and the words of a span.

import { spent } from "@/lib/time";
import type { Span } from "./api";

export type Segment = { x: number; w: number; state: Span["state"]; from: number; to: number; turn?: string };
export type Mark = { x: number; at: number; labelled: boolean };

export const WIDTH = 1000;
const HOUR = 3600_000;

// window is the stretch the strip shows: from the period's start to now, the last `hours` of it on a small screen;
// without a start (one conversation on its own) the last day.
export function window(since: string | undefined, now: number, hours?: number): { from: number; to: number } {
  let from = since ? Date.parse(since) : now - 24 * HOUR;
  if (!Number.isFinite(from) || from >= now) from = now - HOUR;
  if (hours && now - from > hours * HOUR) from = now - hours * HOUR;
  return { from, to: now };
}

// layout places the spans inside the window: one outside it is dropped, one across its edge is trimmed
export function layout(spans: Span[], from: number, to: number, width = WIDTH): Segment[] {
  const scale = width / Math.max(1, to - from);
  const out: Segment[] = [];
  for (const span of spans) {
    const a = Math.max(from, Date.parse(span.from));
    const b = Math.min(to, Date.parse(span.to));
    if (!(b > a)) continue;
    out.push({ x: (a - from) * scale, w: (b - a) * scale, state: span.state, from: a, to: b, turn: span.turn });
  }
  return out;
}

// hourMarks is one mark per whole hour of the clock inside the window, labelled when the hour divides by `every`
export function hourMarks(from: number, to: number, every = 3, width = WIDTH): Mark[] {
  const scale = width / Math.max(1, to - from);
  const first = new Date(from);
  first.setMinutes(0, 0, 0);
  const marks: Mark[] = [];
  for (let t = first.getTime(); t <= to; t += HOUR) {
    if (t < from) continue;
    const hour = new Date(t).getHours();
    marks.push({ x: (t - from) * scale, at: t, labelled: hour % every === 0 });
  }
  return marks;
}

const STATE_WORDS: Record<Span["state"], string> = { working: "Working", waiting: "Waiting for you", idle: "Idle" };

const clock = (at: number) => new Date(at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

// spanWords names a segment for its tooltip: "Working 14:02–14:48, 46 min"
export function spanWords(seg: Pick<Segment, "state" | "from" | "to">): string {
  return `${STATE_WORDS[seg.state]} ${clock(seg.from)}–${clock(seg.to)}, ${spent((seg.to - seg.from) / 1000)}`;
}

// hourLabel is the hour under a mark, in the device's own clock
export const hourLabel = (at: number) => new Date(at).toLocaleTimeString([], { hour: "numeric" });
