// A hue of its own for every remote machine, from its id, so a tile, a sidebar block or a palette row is told apart at a
// glance; the local machine has none. Red and green stay out — they mean blocked and done.

import type { CSSProperties } from "react";

const SPAN = 70 + 180; // 25..95 and 150..330: the bands around red (330..25) and green (95..150) are skipped

export function machineHue(id: string): number | null {
  if (!id || id === "local") return null;
  let hash = 5381;
  for (const ch of id) hash = (hash * 33) ^ ch.codePointAt(0)!;
  const at = Math.abs(hash) % SPAN;
  return at < 70 ? 25 + at : 150 + (at - 70);
}

export const hueColor = (hue: number, alpha = 1) => `hsl(${hue} 55% 58% / ${alpha})`;

// hueStyle is the CSS variable a machine's pieces read, or nothing for the local machine
export function hueStyle(id: string): CSSProperties | undefined {
  const hue = machineHue(id);
  return hue === null ? undefined : ({ "--machine-hue": hue } as CSSProperties);
}
