// The live progress of a turn under way, as Claude Code's own spinner says it — "Refactoring… 2 min 14 s · ↓ 6.8k" — read
// off the pane's screen by GET /api/question; the time runs on from the read a second at a time, so it ticks between reads.

import { tokFine } from "@/features/usage/format";
import { spent } from "@/lib/time";
import type { Progress, Turn } from "./api";

// LiveRead is the turn under way as the screen last showed it: the spinner, when it was read, when the prompt went (unix ms)
export type LiveRead = { progress?: Progress; readAt: number; since: number };

// spinnerSeconds is the turn's time now: the spinner's figure run on from the moment it was read, else the time since
// the prompt the turn answers (the spinner names none for its first seconds); null when neither is known
export function spinnerSeconds(progress: Progress | undefined, readAt: number, now: number, since?: number): number | null {
  if (progress?.seconds !== undefined) return progress.seconds + Math.max(0, Math.floor((now - readAt) / 1000));
  if (since && now >= since) return Math.floor((now - since) / 1000);
  return null;
}

// spinnerWords is the line itself: the verb with its ellipsis, the time, the tokens written, each left out when unknown
export function spinnerWords(verb: string, seconds: number | null, tokens?: number): string {
  const figures = [seconds !== null ? spent(seconds) : "", tokens ? `↓ ${tokFine(tokens)}` : ""].filter(Boolean).join(" · ");
  if (!verb) return figures;
  return figures ? `${verb}… ${figures}` : `${verb}…`;
}

// promptedAt is when the turn under way began: the last prompt of the person's, unix ms; 0 when the conversation has none
export function promptedAt(turns: Turn[]): number {
  for (let i = turns.length - 1; i >= 0; i--) {
    const turn = turns[i]!;
    if (turn.role === "user" && turn.ts) return Date.parse(turn.ts) || 0;
  }
  return 0;
}
