// "Wake a sleeping workspace when I open it": while that switch is on, opening a pane of a workspace asleep in the
// Agents mode wakes it, once per sleep; the switch is read only when such a workspace is on screen. Never for a viewer.

import { useEffect, useRef } from "react";
import type { Machine, Session } from "@/features/roster/api";
import { useQuery } from "@/lib/query";
import { fetchSleepSettings } from "./api";
import { wake } from "./flows";
import { wakeKey } from "./sleep";

export function useWakeOnOpen(on: { machine: Machine; session: Session } | null, enabled: boolean) {
  const asleep = enabled && !!on?.session.asleep;
  const { data } = useQuery(fetchSleepSettings, [], { enabled: asleep });
  const asked = useRef(new Set<string>());
  const key = on && asleep ? wakeKey(on.machine.id, on.session) : "";
  useEffect(() => {
    if (!on || !key || !data?.wake_on_open || on.machine.state !== "connected" || asked.current.has(key)) return;
    asked.current.add(key);
    void wake(on.machine.id, on.session);
  }, [key, data?.wake_on_open]); // eslint-disable-line react-hooks/exhaustive-deps
}
