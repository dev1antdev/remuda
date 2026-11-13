// useScreenRead is the pane's screen as GET /api/question reads it — the dialog, Claude Code's permission mode, its
// spinner — one read for the mode chip and the progress line: every few seconds while the agent works (the spinner's
// figures move; its clock runs on between reads), slower at rest, only while on screen, and at once when work starts.

import { useEffect, useRef } from "react";
import { useLiveConnected } from "@/lib/live";
import { useQuery } from "@/lib/query";
import { fetchScreenRead, type ScreenRead } from "./api";

export const WORKING_EVERY = 5000;
export const TILE_SCREEN_EVERY = 10_000; // a wall tile's, while its agent works

export function useScreenRead(machineId: string, paneId: string, { enabled, working, every: pace }: { enabled: boolean; working: boolean; every?: number }): { read: ScreenRead | null; reload: () => void } {
  const connected = useLiveConnected();
  const every = pace ?? (working ? WORKING_EVERY : connected ? 15_000 : 10_000);
  const { data, reload } = useQuery(() => fetchScreenRead(machineId, paneId), [machineId, paneId], { every, enabled: enabled && !!paneId, keep: true });
  const wasWorking = useRef(working);
  useEffect(() => {
    if (working && !wasWorking.current && enabled) reload();
    wasWorking.current = working;
  }, [working, enabled, reload]);
  return { read: data, reload };
}
