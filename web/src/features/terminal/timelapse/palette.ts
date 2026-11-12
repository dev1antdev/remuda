// The GIF's palette: the colours the recording's cells actually wear — the flavour's terminal palette as the ANSI
// renderer resolved it, the 256-colour and true-colour ones an agent drew in — the background and the ink first, the
// rest by how often they show, at most 256; a colour past those is drawn as the nearest one kept. Pure.

export const MAX_COLOURS = 256;

// Tally counts the colours a recording's cells use
export class Tally {
  private counts = new Map<number, number>();

  add(rgb: number, n = 1) {
    this.counts.set(rgb, (this.counts.get(rgb) ?? 0) + n);
  }

  // palette is the defaults, then the most used, MAX_COLOURS at most
  palette(defaults: number[]): number[] {
    const out = [...new Set(defaults)];
    const rest = [...this.counts.entries()].filter(([rgb]) => !out.includes(rgb)).sort((a, b) => b[1] - a[1] || a[0] - b[0]);
    for (const [rgb] of rest) {
      if (out.length >= MAX_COLOURS) break;
      out.push(rgb);
    }
    return out;
  }
}

// indexer finds a colour's place in the palette: its own, else the nearest one's, worked out once per colour
export function indexer(palette: number[]): (rgb: number) => number {
  const known = new Map<number, number>();
  palette.forEach((rgb, i) => {
    if (!known.has(rgb)) known.set(rgb, i);
  });
  return (rgb) => {
    const hit = known.get(rgb);
    if (hit !== undefined) return hit;
    let best = 0;
    let bestDistance = Infinity;
    palette.forEach((candidate, i) => {
      const distance = distanceOf(rgb, candidate);
      if (distance < bestDistance) {
        best = i;
        bestDistance = distance;
      }
    });
    known.set(rgb, best);
    return best;
  };
}

// distanceOf is how far two colours lie apart, the channels weighted as the eye weighs them
export function distanceOf(a: number, b: number): number {
  const dr = ((a >> 16) & 0xff) - ((b >> 16) & 0xff);
  const dg = ((a >> 8) & 0xff) - ((b >> 8) & 0xff);
  const db = (a & 0xff) - (b & 0xff);
  return 2 * dr * dr + 4 * dg * dg + 3 * db * db;
}

// INK_FROM is how far from the ground toward the ink a glyph's soft edge must reach to count as ink: under half, so a
// thin stroke at a small size stays whole
export const INK_FROM = 0.4;

// isInk says whether a pixel the canvas drew in a cell is the cell's ink or its ground, by how far it lies from the
// ground toward the ink, so the glyphs' soft edges come out sharp in exactly the cell's two colours
export function isInk(r: number, g: number, b: number, ink: number, ground: number): boolean {
  const ir = ((ink >> 16) & 0xff) - ((ground >> 16) & 0xff);
  const ig = ((ink >> 8) & 0xff) - ((ground >> 8) & 0xff);
  const ib = (ink & 0xff) - (ground & 0xff);
  const span = ir * ir + ig * ig + ib * ib;
  if (span === 0) return false;
  const along = (r - ((ground >> 16) & 0xff)) * ir + (g - ((ground >> 8) & 0xff)) * ig + (b - (ground & 0xff)) * ib;
  return along / span >= INK_FROM;
}
