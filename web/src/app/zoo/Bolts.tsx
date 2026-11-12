// Bolts is the lightning over a pen between two creatures whose workspaces change the same files: measured off the
// creatures where they stand in the pen's grid, drawn again when the grid changes size; amber when the lines touch.

import { useLayoutEffect, useState } from "react";
import { between, boltKey, boltPath, type Bolt, type Spot } from "@/features/roster/zooBolts";

type Drawn = { key: string; d: string; lines: boolean };

// BODY is how far a creature's body reaches from its centre, as a share of its box's smaller side
const BODY = 0.38;

export function Bolts({ grid, bolts }: { grid: HTMLDivElement | null; bolts: Bolt[] }) {
  const [drawn, setDrawn] = useState<Drawn[]>([]);
  const key = boltKey(bolts);
  useLayoutEffect(() => {
    const box = grid;
    if (!box || !key) {
      setDrawn([]);
      return;
    }
    const spotOf = (tile: string): Spot | null => {
      const creature = [...box.querySelectorAll<HTMLElement>("[data-zoo-key]")].find((node) => node.dataset.zooKey === tile)?.querySelector(".zoo-open");
      if (!creature) return null;
      const at = creature.getBoundingClientRect();
      const origin = box.getBoundingClientRect();
      return { x: at.left + at.width / 2 - origin.left, y: at.top + at.height / 2 - origin.top, r: BODY * Math.min(at.width, at.height) };
    };
    const measure = () =>
      setDrawn(
        bolts.flatMap((bolt) => {
          const from = spotOf(bolt.from);
          const to = spotOf(bolt.to);
          const ends = from && to ? between(from, to) : null;
          return ends ? [{ key: `${bolt.from}>${bolt.to}`, d: boltPath(...ends), lines: bolt.lines }] : [];
        }),
      );
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(box);
    return () => observer.disconnect();
  }, [grid, key]); // eslint-disable-line react-hooks/exhaustive-deps -- the bolts through their key: the same lightning measures nothing again
  if (drawn.length === 0) return null;
  return (
    <svg className="zoo-bolts" aria-hidden="true">
      {drawn.map((bolt) => (
        <path key={bolt.key} d={bolt.d} className={bolt.lines ? "is-lines" : undefined} />
      ))}
    </svg>
  );
}
