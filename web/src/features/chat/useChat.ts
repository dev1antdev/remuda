// useChat reads a pane's conversation at a pace: the lens polls fast while the agent works, the files rail and Code's
// follow slower, the latter for the last few turns only. While the event socket is up the pane is watched there and a "chat" event reloads at
// once, so the poll slows to a safety net. An unchanged file is a 304 and the turns stay; a change of pane, folder or
// session keeps the turns on screen until the next answer comes, rather than blanking the lens meanwhile.

import { useEffect, useRef } from "react";
import { slowerPoll, useLive, useLiveConnected, watchPane } from "@/lib/live";
import { useQuery, type Query } from "@/lib/query";
import { fetchChat, type ChatResponse } from "./api";
import { keepsTurns } from "./partial";

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
  const held = useRef<ChatResponse | null>(null);
  useEffect(() => {
    etag.current = null;
    held.current = null;
  }, [machineId, paneId, sessionId, dir, last, subagent]);
  const connected = useLiveConnected();
  const query = useQuery<ChatResponse>(
    async () => {
      const res = await fetchChat(machineId, paneId, sessionId, etag.current, dir, last, subagent);
      if (!res) return undefined;
      // the live file read for a moment as if it held nothing: keep the turns on screen and ask again with the same
      // etag, rather than emptying the lens until the file is written whole
      if (keepsTurns(held.current, res.body, sessionId, subagent)) return undefined;
      held.current = res.body;
      etag.current = res.etag;
      return res.body;
    },
    [machineId, paneId, sessionId, dir, last, subagent],
    { every: slowerPoll(every, connected), enabled: active, keep: true },
  );
  const { reload } = query;
  useEffect(() => {
    if (!active || !paneId) return;
    return watchPane(machineId, paneId);
  }, [active, machineId, paneId]);
  useLive("chat", (event) => {
    if (active && paneId && event.machine === machineId && event.pane === paneId) reload();
  });
  // a change of pace (the status flipped) is a look at once; not on mount, the query has just gone out, and coming on screen is useQuery's own
  const mounted = useRef(false);
  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
    if (active) reload();
  }, [every, reload]); // eslint-disable-line react-hooks/exhaustive-deps
  return query;
}
