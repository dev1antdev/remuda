// useLiveScreen reads a pane's screen again and again for the screen terminal: every FAST_MS while the person types
// into it, every SLOW_MS while it is only on screen, never while hidden or while a selection holds it still; an
// unchanged screen is a 304. A send reads it at once and once more a moment later, when the echo has landed.

import { useCallback, useEffect, useRef } from "react";
import { useQuery } from "@/lib/query";
import { fetchScreen } from "../api";

export const FAST_MS = 400;
export const SLOW_MS = 1500;
// ECHO_MS is the second look after a send: a shell or an agent takes a moment to draw what it was sent
export const ECHO_MS = 150;

export function useLiveScreen(machineId: string, paneId: string, { active, fast, held }: { active: boolean; fast: boolean; held: boolean }) {
  const etag = useRef<string | null>(null);
  useEffect(() => {
    etag.current = null;
  }, [machineId, paneId]);
  const screen = useQuery<string>(
    async () => {
      const res = await fetchScreen(machineId, paneId, etag.current);
      if (!res) return undefined;
      etag.current = res.etag;
      return res.text;
    },
    [machineId, paneId],
    { every: fast ? FAST_MS : SLOW_MS, enabled: active && !held },
  );
  const echo = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(echo.current), []);
  const { reload } = screen;
  const afterSend = useCallback(() => {
    reload();
    clearTimeout(echo.current);
    echo.current = setTimeout(reload, ECHO_MS);
  }, [reload]);
  return { text: screen.data, error: screen.error, reload, afterSend };
}
