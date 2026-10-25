// useChat reads a pane's conversation at a pace: the lens polls fast while the agent works, a wall tile slower and
// for the last few turns only. While the event socket is up the pane is watched there and a "chat" event reloads at
// once, so the poll slows to a safety net. An unchanged file is a 304 and the turns stay.

import { useEffect, useRef } from "react";
import { slowerPoll, useLive, useLiveConnected, watchPane } from "@/lib/live";
import { useQuery, type Query } from "@/lib/query";
import { fetchChat, type ChatResponse } from "./api";

export function useChat({ machineId, paneId, sessionId, dir, every, active, last, subagent }: {
  machineId: string;
  paneId: string; // "" with a dir: a transcript on its own
  sessionId: string | null;
  dir: string;
  every: number; // the poll interval, by the pane's status
  active: boolean; // on screen: a hidden chat keeps its turns and waits
  last?: number; // only the last N turns
  subagent?: string; // one subagent's own conversation
}): Query<ChatResponse> {
  const etag = useRef<string | null>(null);
  useEffect(() => {
    etag.current = null;
  }, [machineId, paneId, sessionId, dir, last, subagent]);
  const connected = useLiveConnected();
  const query = useQuery<ChatResponse>(
    async () => {
      const res = await fetchChat(machineId, paneId, sessionId, etag.current, dir, last, subagent);
      if (!res) return undefined;
      etag.current = res.etag;
      return res.body;
    },
    [machineId, paneId, sessionId, dir, last, subagent],
    { every: slowerPoll(every, connected), enabled: active },
  );
  const { reload } = query;
  useEffect(() => {
    if (!active || !paneId) return;
    return watchPane(machineId, paneId);
  }, [active, machineId, paneId]);
  useLive("chat", (event) => {
    if (active && paneId && event.machine === machineId && event.pane === paneId) reload();
  });
  // a change of pace (the status flipped) or coming on screen is a look at once; not on mount, the query has just gone out
  const mounted = useRef(false);
  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
    if (active) reload();
  }, [every, active, reload]);
  return query;
}
