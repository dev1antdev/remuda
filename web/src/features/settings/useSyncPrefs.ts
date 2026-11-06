// Settings that follow the person from device to device. With the switch on, the shared prefs (lib/syncKeys.ts) are
// reconciled with the server's copy — on start, whenever the page comes back into view and every minute while it is
// visible — the newer stamp winning either way; a write made here goes up half a second later. Nothing of it runs
// while the switch is off, and nothing leaves the device that is not on the shared list.

import { useEffect } from "react";
import { applyPref, sharedPrefs, usePref, watchWrites } from "@/lib/prefs";
import { plan, type Stamped } from "@/lib/syncKeys";
import { pageVisible } from "@/lib/visibility";
import { fetchPrefs, putPrefs } from "./api";

export const SYNC_PREF = "prefs.sync";
const POLL = 60_000;
const DEBOUNCE = 500;

export function useSyncPrefs(enabled = true): void {
  const [on] = usePref<boolean>(SYNC_PREF, false);
  useEffect(() => {
    if (!on || !enabled) return;
    let stopped = false;
    let etag = "";
    let pending: Record<string, Stamped> = {};
    let timer: ReturnType<typeof setTimeout> | null = null;

    // apply takes the server's copies that are newer than ours
    const apply = (remote: Record<string, Stamped>) => {
      const { apply: newer, push } = plan(sharedPrefs(), remote, new Date().toISOString());
      for (const [key, { value, at }] of Object.entries(newer)) applyPref(key, value, at);
      return push;
    };

    const push = async () => {
      timer = null;
      const part = pending;
      pending = {};
      if (!Object.keys(part).length) return;
      try {
        const merged = await putPrefs(part);
        if (!stopped) apply(merged);
      } catch {
        pending = { ...part, ...pending }; // the next write or pull tries again
      }
    };

    const schedule = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => void push(), DEBOUNCE);
    };

    const pull = async () => {
      try {
        const answer = await fetchPrefs(etag);
        if (stopped) return;
        if (answer) {
          etag = answer.etag;
          const up = apply(answer.prefs);
          pending = { ...pending, ...up };
        }
        if (Object.keys(pending).length) schedule();
      } catch {
        /* offline: the next pull tries again */
      }
    };

    const stopWrites = watchWrites((key) => {
      const mine = sharedPrefs()[key];
      if (!mine) return;
      pending[key] = mine;
      schedule();
    });
    const onVisible = () => {
      if (pageVisible()) void pull();
    };
    document.addEventListener("visibilitychange", onVisible);
    const poll = setInterval(onVisible, POLL);
    void pull();

    return () => {
      stopped = true;
      stopWrites();
      document.removeEventListener("visibilitychange", onVisible);
      clearInterval(poll);
      if (timer) clearTimeout(timer);
    };
  }, [on, enabled]);
}
