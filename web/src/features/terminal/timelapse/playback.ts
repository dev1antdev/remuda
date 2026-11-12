// The player's arithmetic: every frame's screen put back together from the rows that moved, the frame on screen at a
// moment of the turn, the clock run at 10×, 30× or 60× the turn's own time — or a frame at a time, whatever the gaps —
// the steps from frame to frame, and the GIF's delays at the speed picked. Pure.

import type { RecordingFrame } from "./api";

export type Speed = 10 | 30 | 60 | "frame";
export const SPEEDS: Speed[] = [10, 30, 60, "frame"];
export const DEFAULT_SPEED: Speed = 30;

// FRAME_MS is how long each frame shows by frame; HOLD_MS how long a GIF rests on its last frame before it loops
export const FRAME_MS = 250;
export const HOLD_MS = 2000;
// MIN_DELAY is the shortest GIF delay a browser keeps, in hundredths of a second (shorter ones it draws at a tenth)
export const MIN_DELAY = 2;

// screensOf is every frame's text: the frame before's rows, cut or padded to the frame's count, the moved ones laid over
export function screensOf(frames: RecordingFrame[]): string[] {
  const out: string[] = [];
  let rows: string[] = [];
  for (const frame of frames) {
    const next = rows.slice(0, frame.lines);
    while (next.length < frame.lines) next.push("");
    for (const row of frame.rows ?? []) if (row.line >= 0 && row.line < frame.lines) next[row.line] = row.text;
    rows = next;
    out.push(rows.join("\n"));
  }
  return out;
}

// frameAt is the frame on screen at t: the last one at or before it, the first before it began
export function frameAt(times: number[], t: number): number {
  let lo = 0;
  let hi = times.length - 1;
  if (hi < 0 || t <= times[0]!) return 0;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (times[mid]! <= t) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

// Position is where the player stands: t on the turn's clock, and by frame the time the frame on screen has shown
export type Position = { t: number; carry: number };

// advance moves the player on by dt milliseconds of the person's time; ended once it reached the turn's end
export function advance(pos: Position, dt: number, speed: Speed, times: number[], duration: number): { pos: Position; ended: boolean } {
  if (times.length === 0) return { pos, ended: true };
  if (speed !== "frame") {
    const t = Math.min(duration, pos.t + dt * speed);
    return { pos: { t, carry: 0 }, ended: t >= duration };
  }
  let index = frameAt(times, pos.t);
  let carry = pos.carry + dt;
  while (carry >= FRAME_MS && index < times.length - 1) {
    index++;
    carry -= FRAME_MS;
  }
  const last = index >= times.length - 1;
  return { pos: { t: last ? Math.max(times[index]!, Math.min(duration, pos.t)) : times[index]!, carry: last ? 0 : carry }, ended: last };
}

// stepTo is the time of the frame one before or after the one on screen at t
export function stepTo(times: number[], t: number, by: 1 | -1): number {
  if (times.length === 0) return 0;
  const index = Math.max(0, Math.min(times.length - 1, frameAt(times, t) + by));
  return times[index]!;
}

// gifDelays is how long each frame of a GIF shows, in hundredths of a second: the turn's own gaps at the speed picked,
// rounded on the running total so a long recording keeps its length, never under MIN_DELAY; the last one holds
export function gifDelays(times: number[], speed: Speed): number[] {
  return times.map((at, i) => {
    if (i === times.length - 1) return HOLD_MS / 10;
    if (speed === "frame") return FRAME_MS / 10;
    const from = Math.round(at / speed / 10);
    const to = Math.round(times[i + 1]! / speed / 10);
    return Math.max(MIN_DELAY, Math.min(0xffff, to - from));
  });
}

// playedFor is how long the recording plays at a speed, in the person's milliseconds
export function playedFor(times: number[], duration: number, speed: Speed): number {
  return speed === "frame" ? Math.max(0, times.length - 1) * FRAME_MS : duration / speed;
}
