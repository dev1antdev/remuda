// useZooLife watches the zoo's creatures between roster readings and plays what happened: the resting ones clap for a
// finish, a merged pull request sets off the fireworks, a compaction puffs, and — with the voices on — each calls in its
// species' voice. The first reading after the zoo opens only seeds (features/roster/zooLife.ts).

import { useEffect, useRef, useState } from "react";
import { displayOf, type RadarPrefs } from "@/features/roster/radar";
import type { Tile } from "@/features/roster/wall";
import { CHEER_MS, FIREWORKS_MS, PUFF_MS, happenings, type Seen } from "@/features/roster/zooLife";
import { speciesOf } from "@/features/roster/zooWear";
import { sing } from "./voices";

export type ZooLife = { cheering: string | null; fireworks: boolean; puffs: Set<string> };

export function useZooLife(shown: Tile[], radar: RadarPrefs, now: number, voices: boolean): ZooLife {
  const seen = useRef<Map<string, Seen> | null>(null);
  const [cheer, setCheer] = useState<{ by: string; until: number } | null>(null);
  const [fireworksUntil, setFireworksUntil] = useState(0);
  const [puffs, setPuffs] = useState<Map<string, number>>(() => new Map());
  const [clock, setClock] = useState(() => Date.now());

  useEffect(() => {
    const after = new Map<string, Seen>(shown.map((tile) => [tile.key, { display: displayOf(tile.pane, radar, now), pr: tile.pane.pr?.status?.state ?? "", usage: tile.pane.usage }]));
    const before = seen.current;
    seen.current = after;
    if (!before) return;
    const moves = happenings(before, after);
    const at = Date.now();
    if (moves.finished.length) setCheer({ by: moves.finished[0]!, until: at + CHEER_MS });
    if (moves.merged.length) setFireworksUntil(at + FIREWORKS_MS);
    if (moves.compacted.length) setPuffs((was) => new Map([...was, ...moves.compacted.map((key) => [key, at + PUFF_MS] as const)]));
    if (!voices) return;
    const agentOf = (key: string) => shown.find((tile) => tile.key === key)?.pane.agent ?? "";
    if (moves.merged.length) sing(speciesOf(agentOf(moves.merged[0]!)), "merge");
    else if (moves.finished.length) sing(speciesOf(agentOf(moves.finished[0]!)), "finish");
    else if (moves.asked.length) sing(speciesOf(agentOf(moves.asked[0]!)), "ask");
  }, [shown, radar, now, voices]);

  const ends = [cheer?.until ?? 0, fireworksUntil, ...puffs.values()].filter((end) => end > clock);
  const next = ends.length ? Math.min(...ends) : 0;
  useEffect(() => {
    if (!next) return;
    const id = setTimeout(() => setClock(Date.now()), Math.max(0, next - Date.now()) + 20);
    return () => clearTimeout(id);
  }, [next]);

  const live = new Set([...puffs].filter(([, until]) => until > clock).map(([key]) => key));
  return { cheering: cheer && cheer.until > clock ? cheer.by : null, fireworks: fireworksUntil > clock, puffs: live };
}
