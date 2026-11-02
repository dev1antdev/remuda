// Following the agent in the editor: the file its latest finished edit touched, so the Code mode can open it as it happens.

import type { Part, Turn } from "@/features/chat/api";
import { EDIT_TOOLS, toolPath } from "@/features/chat/footprint";

export type Edit = { id: string; path: string };

// finishedAt is when a call's result came: its start plus what it took, NaN when either is unknown
const finishedAt = (part: Part): number => (part.at && part.took !== undefined ? Date.parse(part.at) + part.took * 1000 : NaN);

// newestEdit is the agent's edit whose result came last in the turns given — calls made side by side finish out of
// order, so by the result's time, and by the order they were made where a time is missing; null when none has one yet
export function newestEdit(turns: Turn[]): Edit | null {
  let newest: Edit | null = null;
  let newestFinished = NaN;
  for (const [i, turn] of turns.entries()) {
    if (turn.role !== "assistant") continue;
    for (const [j, part] of turn.parts.entries()) {
      if (part.type !== "tool" || !EDIT_TOOLS.has(part.tool_name ?? "") || part.output === undefined || part.is_error) continue;
      const path = toolPath(part);
      const finished = finishedAt(part);
      if (!path || finished < newestFinished) continue; // NaN on either side compares false: the later one wins
      newest = { id: part.tool_id ?? `${i}:${j}`, path };
      newestFinished = finished;
    }
  }
  return newest;
}
