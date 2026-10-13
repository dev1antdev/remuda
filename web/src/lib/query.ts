// useQuery is the one way a screen reads the server: a fetch that follows its inputs, polls while it is
// on screen, drops a result that arrives after the inputs changed, and reloads on demand. A fetcher that
// resolves undefined says "nothing changed" (an ETag 304) and the data stays.

import { useCallback, useEffect, useRef, useState } from "react";
import { errMsg } from "./text";

export type Query<T> = {
  data: T | null;
  error: string;
  loading: boolean; // nothing loaded yet and a fetch under way
  reload: () => void;
};

export type QueryOptions = {
  every?: number; // poll this many ms apart while enabled and the tab is visible
  enabled?: boolean; // false pauses the polling and the reloads; the data stays
  keep?: boolean; // changed inputs keep the old data on screen until the new answer comes
};

export function useQuery<T>(fn: () => Promise<T | undefined>, deps: unknown[], { every, enabled = true, keep = false }: QueryOptions = {}): Query<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const seq = useRef(0); // the inputs' generation: an answer to an older one is dropped
  const inflight = useRef(false);
  const again = useRef(false);
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;

  const run = useCallback(async () => {
    if (!enabledRef.current) return;
    if (inflight.current) {
      again.current = true;
      return;
    }
    const mine = seq.current;
    inflight.current = true;
    try {
      const result = await fnRef.current();
      if (mine !== seq.current) return;
      if (result !== undefined) setData(result);
      setError("");
      setLoading(false);
    } catch (e) {
      if (mine !== seq.current) return;
      setError(errMsg(e));
      setLoading(false);
    } finally {
      if (mine === seq.current) {
        inflight.current = false;
        if (again.current) {
          again.current = false;
          void run();
        }
      }
    }
  }, []);

  useEffect(() => {
    seq.current++;
    inflight.current = false;
    again.current = false;
    if (!keep) {
      setData(null);
      setLoading(true);
    }
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(() => {
    if (!every || !enabled) return;
    const tick = () => {
      if (document.visibilityState === "visible") void run();
    };
    const timer = setInterval(tick, every);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", tick);
    };
  }, [every, enabled, run]);

  return { data, error, loading, reload: () => void run() };
}
