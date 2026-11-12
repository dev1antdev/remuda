// The turns of the pane's folder the cockpit kept to be undone, paired with the answers of the conversation on screen,
// so an answer's foot offers Undo this turn; read again when the agent starts or stops and as the turns grow.

import { useCallback, useMemo } from "react";
import { actions, app } from "@/app/store";
import { fetchTurns } from "@/features/undo/api";
import { matchTurns, undoable } from "@/features/undo/undo";
import { useQuery } from "@/lib/query";
import type { Turn } from "./api";

export function useUndoTurns({ machineId, paneId, dir, turns, status, enabled }: { machineId: string; paneId: string; dir: string; turns: Turn[]; status: string; enabled: boolean }) {
  const undone = app.use((s) => s.undone);
  const { data } = useQuery(() => fetchTurns(machineId, dir), [machineId, dir, status, turns.length, undone], { enabled: enabled && !!dir, every: 15000, keep: true });
  const byAnswer = useMemo(() => {
    const ids = new Map<string, string>();
    for (const [answer, kept] of matchTurns(turns, (data?.turns ?? []).filter(undoable), paneId)) ids.set(answer, kept.id);
    return ids;
  }, [data, turns, paneId]);
  const onUndo = useCallback((turn: string) => actions.openDialog({ kind: "undo-turn", machineId, dir, turn }), [machineId, dir]);
  return { byAnswer, onUndo };
}
