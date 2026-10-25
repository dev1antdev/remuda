// Following the agent in the editor: the file its latest finished edit touched, so the Code mode can open it as it happens.

import type { Turn } from "@/features/chat/api";
import { EDIT_TOOLS, toolPath } from "@/features/chat/footprint";

export type Edit = { id: string; path: string };

// newestEdit is the agent's last edit with a result in the turns given — null when none has one yet
export function newestEdit(turns: Turn[]): Edit | null {
  for (let i = turns.length - 1; i >= 0; i--) {
    const turn = turns[i]!;
    if (turn.role !== "assistant") continue;
    for (let j = turn.parts.length - 1; j >= 0; j--) {
      const part = turn.parts[j]!;
      if (part.type !== "tool" || !EDIT_TOOLS.has(part.tool_name ?? "") || part.output === undefined || part.is_error) continue;
      const path = toolPath(part);
      if (path) return { id: part.tool_id ?? `${i}:${j}`, path };
    }
  }
  return null;
}
