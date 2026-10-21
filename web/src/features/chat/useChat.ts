// useChat reads a pane's conversation at a pace: the lens polls fast while the agent works, a wall tile slower and
// for the last few turns only. An unchanged file is a 304 and the turns stay.

import { useEffect, useRef } from "react";
import { useQuery, type Query } from "@/lib/query";
import { fetchChat, type ChatResponse } from "./api";

export function useChat({ machineId, paneId, sessionId, dir, every, active, last }: {
  machineId: string;
  paneId: string; // "" with a dir: a transcript on its own
  sessionId: string | null;
  dir: string;
  every: number; // the poll interval, by the pane's status
  active: boolean; // on screen: a hidden chat keeps its turns and waits
  last?: number; // only the last N turns
}): Query<ChatResponse> {
  const etag = useRef<string | null>(null);
  useEffect(() => {
    etag.current = null;
  }, [machineId, paneId, sessionId, dir, last]);
  const query = useQuery<ChatResponse>(
    async () => {
      const res = await fetchChat(machineId, paneId, sessionId, etag.current, dir, last);
      if (!res) return undefined;
      etag.current = res.etag;
      return res.body;
    },
    [machineId, paneId, sessionId, dir, last],
    { every, enabled: active },
  );
  // a change of pace (the status flipped) or coming on screen is a look at once; not on mount, the query has just gone out
  const mounted = useRef(false);
  const { reload } = query;
  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
    if (active) reload();
  }, [every, active, reload]);
  return query;
}
