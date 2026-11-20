// useTerminalFind drives the find over the scrollback from the find bar's store field while the terminal lens is on
// screen. herdr keeps the rows: the query goes to the server, which finds it in the pane's last rows and says where
// the viewport stands; the match asked for — the first for a new query, the one the bar stepped to — is scrolled
// into view through the pane itself when it is off the screen, then marked there by xterm's search. On remuda-core
// (core) xterm holds the terminal's own rows, so the find looks through them in the page and scrolls xterm alone;
// when none of them has the query, the server reads the terminal's older rows and says if it is only there, where
// the screen cannot show it. A bar closed on screen with a keyboard gives the terminal the focus back. The chat's own
// find runs only on its lens.

import { useEffect, useRef } from "react";
import { actions, app } from "@/app/store";
import { isTouch } from "@/lib/prefs";
import { tn } from "@/lib/i18n";
import { errMsg } from "@/lib/text";
import { findScrollback, scrollPane } from "./api";
import type { TermApi } from "./engine/types";
import { blankTail, clampAt, fromBottom, offsetFor, onScreen, SCROLLBACK_ROWS, viewportRow } from "./scrollback";

// DEBOUNCE_MS lets a query settle before the rows are read for it
const DEBOUNCE_MS = 150;
// FALLBACK_ROWS stands in for the viewport's height when herdr has not said it yet
const FALLBACK_ROWS = 24;

export function useTerminalFind(api: TermApi | null, active: boolean, machineId: string, paneId: string, core = false) {
  const open = app.use((s) => s.find !== null);
  const query = app.use((s) => s.find?.query ?? "");
  const steps = app.use((s) => s.find?.steps ?? 0);
  const at = app.use((s) => s.find?.at ?? 0);
  const on = active && open && api !== null;
  const run = useRef(0);
  // the core find's latest count, and the one read of older rows per pane and query that every step of it shares
  const counted = useRef(0);
  const olderRead = useRef<{ key: string; matches: Promise<number>; told: boolean } | null>(null);

  // older says, once a pane and query, that the query's matches are only in rows the terminal no longer holds for the
  // screen: the server reads them once, and whichever run of that query is current when it answers, with the count
  // still zero, tells
  const older = async (q: string, id: number) => {
    const key = `${machineId}|${paneId}|${q}`;
    if (olderRead.current?.key !== key) {
      olderRead.current = { key, matches: findScrollback(machineId, paneId, q, SCROLLBACK_ROWS).then((f) => f.matches.length), told: false };
    }
    const read = olderRead.current;
    let n = 0;
    try {
      n = await read.matches;
    } catch (e) {
      if (id === run.current && !read.told) actions.notify(errMsg(e), { error: true });
      read.told = true;
      return;
    }
    if (id !== run.current || read.told || !n || counted.current !== 0) return;
    read.told = true;
    actions.notify(tn(n, "{n} match is only in older rows, which this terminal no longer shows", "{n} matches are only in older rows, which this terminal no longer shows"));
  };

  // show finds the query in the pane's rows, counts for the bar, brings the match asked for on screen and marks it
  const show = async (q: string, index: number, step: 0 | 1 | -1) => {
    if (!api) return;
    const id = ++run.current;
    try {
      if (core && api.search.find) {
        api.search.find(q, step, ({ total, at: shown }) => {
          if (id !== run.current) return;
          counted.current = total;
          actions.foundMatches(total, shown);
          if (!total) void older(q, id);
        });
        return;
      }
      const found = await findScrollback(machineId, paneId, q, SCROLLBACK_ROWS);
      if (id !== run.current) return;
      const total = found.matches.length;
      const i = clampAt(index, total);
      actions.foundMatches(total, i);
      if (!total) {
        api.search.clear();
        return;
      }
      const match = found.matches[i]!;
      const above = fromBottom(match.row, found.rows, blankTail(found.rows, found.truncated, found.scroll));
      let scroll = found.scroll;
      let rows = scroll?.viewport_rows || FALLBACK_ROWS;
      let row = viewportRow(above, scroll?.offset_from_bottom ?? 0, rows);
      if (!onScreen(row, rows)) {
        const want = offsetFor(above, rows);
        const moved = await scrollPane(machineId, paneId, want);
        if (id !== run.current) return;
        scroll = moved.scroll ?? scroll;
        rows = scroll?.viewport_rows || rows;
        row = viewportRow(above, scroll?.offset_from_bottom ?? want, rows);
      }
      api.search.show(q, row, match.col);
    } catch (e) {
      if (id === run.current) actions.notify(errMsg(e), { error: true });
    }
  };

  // the query as typed, once it settles
  useEffect(() => {
    if (!on) return;
    if (!query) {
      run.current++;
      api.search.clear();
      actions.foundMatches(0, 0);
      return;
    }
    const timer = setTimeout(() => void show(query, 0, 0), DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [on, api, query, machineId, paneId, core]); // eslint-disable-line react-hooks/exhaustive-deps

  // a step of the bar: the rows read again, so a log still growing keeps its places
  const stepped = useRef(steps);
  useEffect(() => {
    const was = stepped.current;
    stepped.current = steps;
    if (!on || steps === was || !query) return;
    void show(query, at, app.get().find?.by ?? 1);
  }, [on, api, steps, at, query, machineId, paneId, core]); // eslint-disable-line react-hooks/exhaustive-deps

  // the marks go when the bar closes or the lens leaves; closed on screen with a keyboard, the terminal takes the focus
  const wasOn = useRef(on);
  useEffect(() => {
    const was = wasOn.current;
    wasOn.current = on;
    if (on || !was || !api) return;
    run.current++;
    olderRead.current = null;
    api.search.clear();
    if (active && !open && !isTouch) api.focus();
  }, [on, api, active, open]);
}
