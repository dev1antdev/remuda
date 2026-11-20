// The grid a viewer is told on remuda-core: a viewer that keeps the size, or only watches, gets the terminal's grid as a
// message of its own, in order with the bytes (a text frame on the socket, X-Remuda-Grid on HTTP) — never a sequence in
// the bytes, which the program itself could print. It is applied once the bytes before it are parsed and before the ones
// after it, through xterm's own write queue, a hidden lens's too.

import type { Terminal } from "@xterm/xterm";

// validGrid keeps a grid within what a terminal holds: nothing a stray message could stall the page with.
export function validGrid(cols: number, rows: number): boolean {
  return Number.isInteger(cols) && Number.isInteger(rows) && cols >= 2 && cols <= 1000 && rows >= 1 && rows <= 500;
}

// atGrid sets the grid in xterm's write queue, after what was written before it, and hands it to onGrid.
export function atGrid(term: Terminal, cols: number, rows: number, onGrid: (cols: number, rows: number) => void): void {
  if (!validGrid(cols, rows)) return;
  term.write("", () => {
    if (term.cols !== cols || term.rows !== rows) term.resize(cols, rows);
    onGrid(cols, rows);
  });
}
