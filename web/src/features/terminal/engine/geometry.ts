// The grid's size: fit resizes the shared pane to this screen, keep attaches at the pane's own grid and
// shrinks the font to fit the width, scrolling when it can't.

import type { FitAddon } from "@xterm/addon-fit";
import type { Terminal } from "@xterm/xterm";
import type { SizeMode } from "./types";

export const MIN_FONT = 8;
const PAD = 8; // the mount's padding, both sides

export type Geometry = {
  mode: SizeMode;
  cols: number; // the pane's own grid, 0 when unknown
  rows: number;
  minCols: number; // fit mode: never narrower than this many columns (0 = off)
  fontSize: number; // preferred size in fit mode
};

// cellSize reads the cell box in px from xterm's private _renderService; before the first draw, an estimate.
export function cellSize(term: Terminal): { w: number; h: number } {
  const core = (term as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { width: number; height: number } } } } } })._core;
  const c = core?._renderService?.dimensions?.css?.cell;
  const size = term.options.fontSize!;
  return { w: c?.width || size * 0.6, h: c?.height || size * 1.2 };
}

// keepFont is the font size that fits cols columns in avail px: the preferred size, smaller when it must, never under MIN_FONT.
export function keepFont(avail: number, preferred: number, cols: number, cellWidth: number, fontSize: number): number {
  return Math.max(MIN_FONT, Math.min(preferred, Math.floor((avail * fontSize) / (cols * cellWidth))));
}

// thumbFont is the size at which cols columns fill avail pixels, given a cell's width at fontSize: a screen drawn
// small, never larger than fontSize, in half-pixel steps so a resize by a pixel redraws nothing
export function thumbFont(avail: number, cols: number, cellWidth: number, fontSize: number): number {
  const fit = (avail * fontSize) / (Math.max(1, cols) * cellWidth);
  return Math.max(2, Math.min(fontSize, Math.floor(fit * 2) / 2));
}

// applyGeometry sizes the grid: fit → to the mount (at least minCols wide, the mount scrolling sideways),
// keep → the pane's own grid with the font shrunk to fit the width. False when the mount is hidden.
export function applyGeometry(term: Terminal, fit: FitAddon, mount: HTMLElement, g: Geometry): boolean {
  if (mount.clientWidth === 0) return false; // hidden: nothing to measure
  mount.classList.remove("keep", "wide");
  if (g.mode === "keep" && g.cols > 0 && g.rows > 0) {
    const size = keepFont(mount.clientWidth - PAD, g.fontSize, g.cols, cellSize(term).w, term.options.fontSize!);
    if (term.options.fontSize !== size) term.options.fontSize = size;
    if (term.cols !== g.cols || term.rows !== g.rows) term.resize(g.cols, g.rows);
    mount.classList.add("keep");
  } else {
    if (term.options.fontSize !== g.fontSize) term.options.fontSize = g.fontSize;
    const fits = fit.proposeDimensions();
    if (g.minCols > 0 && fits && fits.cols < g.minCols) {
      // the class brings the scrollbar first, so clientHeight already excludes it when the rows are measured
      mount.classList.add("wide");
      const rows = Math.max(5, Math.floor((mount.clientHeight - PAD) / cellSize(term).h));
      if (term.cols !== g.minCols || term.rows !== rows) term.resize(g.minCols, rows);
    } else {
      fit.fit();
    }
  }
  return true;
}

// keepFollower keeps a keep-mode grid taller than the screen at its bottom unless the reader scrolled up.
export function keepFollower(mount: HTMLElement): () => void {
  let queued = false;
  return () => {
    if (queued || !mount.classList.contains("keep")) return;
    queued = true;
    requestAnimationFrame(() => {
      queued = false;
      const nearBottom = mount.scrollHeight - mount.scrollTop - mount.clientHeight < 80;
      if (nearBottom) mount.scrollTop = mount.scrollHeight;
    });
  };
}
