// The activity panel: what happened across the agents, as the server's feed keeps it.

import { getJSON } from "@/lib/http";

export type Entry = { id: number; at: string; kind: string; machine: string; pane?: string; title: string; body?: string; quiet?: boolean };

export const fetchFeed = (limit = 100): Promise<Entry[]> => getJSON<{ entries: Entry[] }>(`/api/feed?limit=${limit}`).then((r) => r.entries);
