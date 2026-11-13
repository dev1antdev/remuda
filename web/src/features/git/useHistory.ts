// The lens's reads: the history (as many pages as are shown, read again whole only when the refs' fingerprint moved),
// and where the checkout stands, read every few seconds while the lens is on screen — the uncommitted row and a stopped
// merge follow it at once, the graph only when a ref moved.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { readLog, readState, type Scope } from "./api";
import { historyOf, PAGE, withPage, withUncommitted, type History } from "./history";

const STATE_EVERY = 4000;
const MAX_PAGES = 30; // a reread asks for the pages shown, at most this many

export function useHistory(machineId: string, dir: string, scope: Scope, active: boolean) {
  const pages = useRef(1);
  const [more, setMore] = useState<{ busy: boolean; error: string }>({ busy: false, error: "" });
  const key = `${machineId}|${dir}|${scope.all}|${scope.remotes}`;
  const shownKey = useRef(key);
  if (shownKey.current !== key) {
    shownKey.current = key;
    pages.current = 1;
  }
  const log = useQuery<History>(
    async () => {
      const read = await readLog(machineId, dir, scope, 0, Math.min(pages.current, MAX_PAGES) * PAGE);
      return historyOf(read, read.dirty > 0);
    },
    [machineId, dir, scope.all, scope.remotes],
    { enabled: active && !!dir, keep: true },
  );
  const state = useQuery(() => readState(machineId, dir), [machineId, dir], { every: STATE_EVERY, enabled: active && !!dir && !!log.data?.log.repo });
  const fingerprint = state.data?.fingerprint;
  const drawn = log.data?.log.fingerprint;
  const { reload, fetching, mutate } = log;
  const reloadState = state.reload;
  // a ref moved, here or anywhere: the history is read again, once for each fingerprint the state reads
  const askedFor = useRef("");
  useEffect(() => {
    if (!fingerprint || !drawn || fingerprint === drawn || fetching || askedFor.current === fingerprint) return;
    askedFor.current = fingerprint;
    reload();
  }, [fingerprint, drawn, fetching, reload]);

  const history = useMemo(() => (log.data && state.data?.repo ? withUncommitted(log.data, state.data.dirty > 0) : log.data), [log.data, state.data]);

  const loadMore = useCallback(async () => {
    const shown = log.data;
    if (!shown || more.busy || !shown.log.more) return;
    setMore({ busy: true, error: "" });
    try {
      const page = await readLog(machineId, dir, scope, shown.commits.length, PAGE);
      pages.current = Math.ceil((shown.commits.length + page.commits.length) / PAGE);
      const next = withPage(shown, page);
      if (next) mutate(() => next);
      else reload();
      setMore({ busy: false, error: "" });
    } catch (e) {
      setMore({ busy: false, error: errMsg(e) });
    }
  }, [log.data, more.busy, machineId, dir, scope, mutate, reload]);

  const refresh = useCallback(() => {
    reload();
    reloadState();
  }, [reload, reloadState]);

  return { history, state: state.data ?? log.data?.log ?? null, error: log.error, loading: log.loading, fetching: fetching || more.busy, more, loadMore, refresh };
}
