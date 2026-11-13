// A turn's tool calls drawn as cards: every run of consecutive calls is one bordered card, a line a call with how it went —
// done, failed, still running — what it was about, how long it took. After Orchi's chat. Pure; ToolCard.tsx draws it.

import type { Part } from "./api";

export type Indexed = { part: Part; index: number };

// Piece is what a turn draws in order: a part on its own, or a card of calls (the thinking between them riding along)
export type Piece = { kind: "part"; item: Indexed } | { kind: "card"; items: Indexed[] };

// pieces groups a turn's parts: a run of tool calls with nothing but thinking between them is one card; the thinking
// before the first call or after the last stays outside, as does every text and message
export function pieces(parts: Part[]): Piece[] {
  const out: Piece[] = [];
  let i = 0;
  while (i < parts.length) {
    if (parts[i]!.type !== "tool") {
      out.push({ kind: "part", item: { part: parts[i]!, index: i } });
      i++;
      continue;
    }
    let end = i;
    for (let j = i + 1; j < parts.length; j++) {
      const type = parts[j]!.type;
      if (type === "tool") end = j;
      else if (type !== "thinking") break;
    }
    const items: Indexed[] = [];
    for (let j = i; j <= end; j++) items.push({ part: parts[j]!, index: j });
    out.push({ kind: "card", items });
    i = end + 1;
  }
  return out;
}

export type CallState = "done" | "failed" | "running" | "unanswered";

// callState is how a call went: its result in, failed, still running while the agent works, else never answered (a turn
// cut off, a call refused before it ran)
export function callState(part: Part, live: boolean): CallState {
  if (part.is_error) return "failed";
  if (part.output !== undefined) return "done";
  return live ? "running" : "unanswered";
}
