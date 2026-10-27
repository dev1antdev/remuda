// What a turn's tool calls add up to: how many failed, how many repeated an earlier call of the same turn exactly.

import type { Part, Turn } from "./api";
import { t } from "@/lib/i18n";

export type CallStats = { failed: number; repeated: number };

// callStats counts the turn's failed calls and its repeats: a call whose tool and input match an earlier one of the turn
// to the byte — a file read again, the same search twice, a failing command run again
export function callStats(turn: Turn): CallStats {
  const seen = new Set<string>();
  const stats: CallStats = { failed: 0, repeated: 0 };
  for (const part of turn.parts) {
    if (part.type !== "tool") continue;
    if (part.is_error) stats.failed++;
    const key = callKey(part);
    if (seen.has(key)) stats.repeated++;
    else seen.add(key);
  }
  return stats;
}

function callKey(part: Part): string {
  const input = typeof part.input === "string" ? part.input : JSON.stringify(part.input ?? null);
  return `${part.tool_name ?? ""}\u0000${input}`;
}

// callWords is the foot's phrase for the stats, "" when nothing went wrong
export function callWords(stats: CallStats): string {
  const words: string[] = [];
  if (stats.failed) words.push(t("{n} failed", { n: stats.failed }));
  if (stats.repeated) words.push(t("{n} repeated", { n: stats.repeated }));
  return words.join(" · ");
}
