// The roster, read once for the whole page: reloaded when the event socket says it moved and polled as the safety
// net — every fifteen seconds with the socket up, every three without — with an ETag so an unchanged roster costs
// nothing and re-renders nothing. Every screen reads it through useRoster.

import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { setRosterRefresh } from "@/app/act";
import { startLive, useLive, useLiveConnected } from "@/lib/live";
import { useQuery } from "@/lib/query";
import type { BudgetStatus } from "@/features/usage/api";
import type { Retirement } from "@/features/usage/retired";
import { splitConcierge, type ConciergeEntry } from "@/features/concierge/concierge";
import { fetchRoster, type AgentKind, type Latest, type Machine, type Roster, type Tailscale } from "./api";
import { hostMachine } from "./server";

export type RosterState = {
  machines: Machine[]; // the fleet without the concierge's workspace, which its place alone reads
  concierges: ConciergeEntry[]; // the concierge's workspace on each machine where one stands
  agents: AgentKind[];
  latest?: Latest;
  tailscale?: Tailscale;
  version?: string; // the server's
  feed?: number; // the newest activity entry: past the one seen here, the bell has news
  budgets?: BudgetStatus[]; // the spending limits with what their periods have cost so far
  jiraSite?: string; // the person's Jira site, when one is set: a task can then come from a Jira issue
  server?: Machine; // the host of a remuda serving the page only (remuda -server), no machine of the fleet: the Server place's
  host?: Machine; // the machine that serves the page, in the fleet or not: its own updates (a release) are offered on it
  retirements: Retirement[]; // the models and modes being retired, the server's table, for a form that edits a model
  offers?: string[]; // the plan limits' open offers by id: a card or schedule the limit held, to run on the other vendor
  withdrawn?: string[]; // the tags of the notifications taken back within a day: this browser closes them where they show
  offline: boolean;
  stale: string | null; // the server's version when this page is older
  viewOnly: boolean; // a viewer link signed this page in: it draws, and hides every control that would type or change
  refresh: () => void;
};

const empty: RosterState = { machines: [], concierges: [], agents: [], retirements: [], offline: false, stale: null, viewOnly: false, refresh: () => {} };
const RosterContext = createContext<RosterState>(empty);

export function RosterProvider({ children }: { children: ReactNode }) {
  const etag = useRef<string | null>(null);
  const [stale, setStale] = useState<string | null>(null);
  const [viewOnly, setViewOnly] = useState(false);
  const connected = useLiveConnected();
  const { data, error, reload } = useQuery<Roster>(
    async () => {
      const res = await fetchRoster(etag.current);
      if (res.version && res.version !== __REMUDA_VERSION__ && __REMUDA_VERSION__ !== "dev") setStale(res.version);
      if (res.viewOnly) setViewOnly(true);
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
  const value = useMemo<RosterState>(() => {
    const server = data?.server ? hostMachine(data.server) : undefined;
    const { machines, concierges } = splitConcierge(data?.machines ?? []);
    return {
      server,
      host: machines.find((m) => m.id === "local") ?? server,
      machines,
      concierges,
      agents: data?.agents ?? [],
      latest: data?.latest,
      tailscale: data?.tailscale,
      version: data?.version,
      feed: data?.feed,
      budgets: data?.budgets,
      jiraSite: data?.jira,
      retirements: data?.retirements ?? [],
      offers: data?.offers,
      withdrawn: data?.withdrawn,
      offline: !!error,
      stale,
      viewOnly,
      refresh: reload,
    };
  }, [data, error, stale, viewOnly, reload]);
  return <RosterContext.Provider value={value}>{children}</RosterContext.Provider>;
}

export function useRoster(): RosterState {
  return useContext(RosterContext);
}
