// useScreenSearch asks the server where a text stood on the recorded screens of a scope — the player's pane and folder,
// or every recording from the search panel — once the typing has settled, the request before it called off so the
// server stops reading for a query nobody waits on.

import { useEffect, useRef } from "react";
import { useQuery } from "@/lib/query";
import { useDebounced } from "@/ui";
import { searchScreens, type ScreenResult, type ScreenScope } from "./api";
import { MIN_MARKED } from "./mark";

const SETTLE_MS = 300;

export function useScreenSearch(text: string, scope: ScreenScope, enabled = true) {
  const query = useDebounced(text.trim(), SETTLE_MS);
  const short = [...query].length < MIN_MARKED;
  const asked = useRef<AbortController | null>(null);
  useEffect(() => () => asked.current?.abort(), []);
  const found = useQuery<ScreenResult | null>(
    () => {
      asked.current?.abort();
      if (short) return Promise.resolve(null);
      const mine = new AbortController();
      asked.current = mine;
      return searchScreens(query, scope, mine.signal);
    },
    [query, short, scope.machine, scope.pane, scope.dir, scope.days],
    { enabled },
  );
  return { ...found, query: short ? "" : query, settled: query === text.trim() };
}
