// Mouse reports as the app asked for them, synthesized by hand: the cell under a pointer, one report in the
// app's encoding (SGR, else X10), the wheel as ticks that add up like a native terminal's.

import type { Terminal } from "@xterm/xterm";
import { cellSize } from "./geometry";
import type { Modes } from "./modes";

export type Cell = { col: number; row: number }; // 1-based
export type Point = { clientX: number; clientY: number };

// report encodes one mouse report: SGR (1006), else X10, whose release is button 3 and whose coordinates stop at 223.
export function report(sgr: boolean, code: number, c: Cell, release: boolean): string {
  return sgr
    ? `\x1b[<${code};${c.col};${c.row}${release ? "m" : "M"}`
    : "\x1b[M" + String.fromCharCode(32 + (release ? (code & ~3) | 3 : code), 32 + Math.min(c.col, 223), 32 + Math.min(c.row, 223));
}

export const modifierBits = (e: MouseEvent) => (e.shiftKey ? 4 : 0) + (e.altKey ? 8 : 0) + (e.ctrlKey ? 16 : 0);

export type WheelAcc = { x: number; y: number };

// wheelTicks turns wheel movement into ticks as a native terminal does: the pixels add up in acc, one tick per
// unit (a cell's height, or two cells' width sideways); a turn the other way starts the count over.
export function wheelTicks(acc: WheelAcc, delta: number, sideways: boolean, unit: number): number {
  const axis = sideways ? "x" : "y";
  if (Math.sign(delta) !== Math.sign(acc[axis])) acc[axis] = 0;
  acc[axis] += delta;
  const ticks = Math.trunc(acc[axis] / unit);
  acc[axis] -= ticks * unit;
  return ticks;
}

export type Reporter = {
  cellAt: (p: Point) => Cell | null;
  wheel: (e: WheelEvent, sideways: boolean) => number; // the ticks this wheel event amounts to
  scroll: (ticks: number, sideways: boolean, cell: Cell, mods?: number) => void;
  click: (code: number, cell: Cell) => void;
};

export function createReporter(term: Terminal, modes: Modes, sendInput: (data: string) => void): Reporter {
  const acc: WheelAcc = { x: 0, y: 0 };
  // cellAt is the grid cell under a pointer, 1-based.
  const cellAt = (p: Point): Cell | null => {
    const screen = term.element?.querySelector(".xterm-screen") as HTMLElement | null;
    if (!screen) return null;
    const r = screen.getBoundingClientRect();
    const { w, h } = cellSize(term);
    return {
      col: Math.max(1, Math.min(term.cols, Math.floor((p.clientX - r.left) / w) + 1)),
      row: Math.max(1, Math.min(term.rows, Math.floor((p.clientY - r.top) / h) + 1)),
    };
  };
  const wheel = (e: WheelEvent, sideways: boolean) => {
    const { w, h } = cellSize(term);
    let delta = sideways ? e.deltaX : e.deltaY;
    if (e.deltaMode === 1) delta *= h; // lines
    else if (e.deltaMode === 2) delta *= h * term.rows; // pages
    return wheelTicks(acc, delta, sideways, sideways ? w * 2 : h);
  };
  const scroll = (ticks: number, sideways: boolean, cell: Cell, mods = 0) => {
    if (!ticks) return;
    const code = sideways ? (ticks < 0 ? 66 : 67) : ticks < 0 ? 64 : 65;
    sendInput(report(modes.sgrMouse(), code + mods, cell, false).repeat(Math.min(40, Math.abs(ticks))));
  };
  const click = (code: number, cell: Cell) => sendInput(report(modes.sgrMouse(), code, cell, false) + report(modes.sgrMouse(), code, cell, true));
  return { cellAt, wheel, scroll, click };
}
