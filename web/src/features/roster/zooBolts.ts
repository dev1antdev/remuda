// The collision radar in the zoo: two creatures whose workspaces change the same files of one repository are joined by
// a lightning when they stand in one pen, and each wears a small bolt when they stand in different pens (or the other
// is not on the wall). The pure part, tested; app/zoo/Bolts.tsx draws the lightning between the creatures it measures.

import type { Collision } from "./api";

type Standing = { key: string; machine: { id: string }; session: { id: string; collisions?: Collision[] } };

// Bolt is a lightning between two creatures of one pen, by their tiles' keys; lines when the same lines touch
export type Bolt = { from: string; to: string; lines: boolean };

// zooBolts is the lightning a pen's creatures share and the bolt each creature wears alone (true when the lines touch):
// a pair of workspaces once, between the first creature of each
export function zooBolts<T extends Standing>(tiles: T[]): { bolts: Bolt[]; marked: Map<string, boolean> } {
  const firstOf = new Map<string, T>();
  for (const tile of tiles) {
    const workspace = `${tile.machine.id}|${tile.session.id}`;
    if (!firstOf.has(workspace)) firstOf.set(workspace, tile);
  }
  const bolts = new Map<string, Bolt>();
  const marked = new Map<string, boolean>();
  for (const [workspace, tile] of firstOf) {
    for (const collision of tile.session.collisions ?? []) {
      const peer = firstOf.get(`${collision.with.machine}|${collision.with.workspace}`);
      if (peer && peer.machine.id === tile.machine.id) {
        const [from, to] = [tile.key, peer.key].sort() as [string, string];
        const pair = `${from}>${to}`;
        bolts.set(pair, { from, to, lines: (bolts.get(pair)?.lines ?? false) || collision.lines });
        continue;
      }
      for (const member of tiles) {
        if (`${member.machine.id}|${member.session.id}` === workspace) marked.set(member.key, (marked.get(member.key) ?? false) || collision.lines);
      }
    }
  }
  return { bolts: [...bolts.values()], marked };
}

// boltKey is a pen's lightning as one string, so a redraw that changes none measures nothing again
export function boltKey(bolts: Bolt[]): string {
  return bolts.map((bolt) => `${bolt.from}>${bolt.to}${bolt.lines ? "!" : ""}`).join(",");
}

// KINKS is how many times a lightning turns between two creatures, AMPLITUDE how far it strays from the line, in px
export const KINKS = 5;
export const AMPLITUDE = 7;

export type Spot = { x: number; y: number; r: number }; // a creature's centre and how far its body reaches

// between is where a lightning from one creature to another starts and ends: at the edge of each body, not over their
// faces; null when they stand too close for one
export function between(a: Spot, b: Spot): [number, number, number, number] | null {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const length = Math.hypot(dx, dy);
  if (length <= a.r + b.r) return null;
  const [ux, uy] = [dx / length, dy / length];
  return [a.x + ux * a.r, a.y + uy * a.r, b.x - ux * b.r, b.y - uy * b.r];
}

// boltPath is an SVG path from one point to another that zigzags across the straight line between them
export function boltPath(ax: number, ay: number, bx: number, by: number, kinks = KINKS, amplitude = AMPLITUDE): string {
  const dx = bx - ax;
  const dy = by - ay;
  const length = Math.hypot(dx, dy) || 1;
  const [nx, ny] = [-dy / length, dx / length];
  const points: string[] = [`M${round(ax)} ${round(ay)}`];
  for (let i = 1; i <= kinks; i++) {
    const at = i / (kinks + 1);
    const side = i % 2 === 1 ? amplitude : -amplitude;
    points.push(`L${round(ax + dx * at + nx * side)} ${round(ay + dy * at + ny * side)}`);
  }
  points.push(`L${round(bx)} ${round(by)}`);
  return points.join(" ");
}

const round = (value: number) => Math.round(value * 10) / 10;
