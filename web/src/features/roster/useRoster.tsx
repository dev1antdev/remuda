// The roster, read once for the whole page: reloaded when the event socket says it moved and polled as the safety
// net — every fifteen seconds with the socket up, every three without — with an ETag so an unchanged roster costs
// nothing and re-renders nothing. Every screen reads it through useRoster.

import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { setRosterRefresh } from "@/app/act";
import { startLive, useLive, useLiveConnected } from "@/lib/live";
import { useQuery } from "@/lib/query";
import { fetchRoster, type AgentKind, type Latest, type Machine, type Roster, type Tailscale } from "./api";

export type RosterState = {
  machines: Machine[];
  agents: AgentKind[];
  latest?: Latest;
  tailscale?: Tailscale;
  version?: string; // the server's
  feed?: number; // the newest activity entry: past the one seen here, the bell has news
  offline: boolean;
  stale: string | null; // the server's version when this page is older
  refresh: () => void;
};

const empty: RosterState = { machines: [], agents: [], offline: false, stale: null, refresh: () => {} };
const RosterContext = createContext<RosterState>(empty);

export function RosterProvider({ children }: { children: ReactNode }) {
  const etag = useRef<string | null>(null);
  const [stale, setStale] = useState<string | null>(null);
  const connected = useLiveConnected();
  const { data, error, reload } = useQuery<Roster>(
    async () => {
      const res = await fetchRoster(etag.current);
      if (res.version && res.version !== __REMUDA_VERSION__ && __REMUDA_VERSION__ !== "dev") setStale(res.version);
      if (!res.roster) return undefined; // unchanged
      etag.current = res.etag;
      return res.roster;
    },
    [],
    { every: connected ? 15000 : 3000 },
  );
  useEffect(() => setRosterRefresh(reload), [reload]);
  useEffect(() => {
    startLive();
  }, []);
  useLive("roster", reload);
  const value = useMemo<RosterState>(
    () => ({
      machines: data?.machines ?? [],
      agents: data?.agents ?? [],
      latest: data?.latest,
      tailscale: data?.tailscale,
      version: data?.version,
      feed: data?.feed,
      offline: !!error,
      stale,
      refresh: reload,
    }),
    [data, error, stale, reload],
  );
  return <RosterContext.Provider value={value}>{children}</RosterContext.Provider>;
}

export function useRoster(): RosterState {
  return useContext(RosterContext);
}
