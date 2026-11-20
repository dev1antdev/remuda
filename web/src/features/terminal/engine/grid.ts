// The grid a viewer is told in its own bytes: on remuda-core a viewer that keeps the size, or only watches, gets the
// terminal's grid as xterm's own CSI 8;rows;cols t ahead of the paint drawn at it. xterm calls a handler of that
// sequence only with windowOptions.setWinSizeChars on (WINDOW_OPTIONS), and the grid is set right there, while the
// bytes are parsed, so what follows lands at it — a hidden lens too, which measures nothing.

import type { IDisposable, Terminal } from "@xterm/xterm";

export const WINDOW_OPTIONS = { setWinSizeChars: true };

// followGrid sets the grid each CSI 8 t names while follows() says so, and hands it to onGrid; any other time the
// sequence is swallowed, as xterm does without the option.
export function followGrid(term: Terminal, follows: () => boolean, onGrid: (cols: number, rows: number) => void): IDisposable {
  return term.parser.registerCsiHandler({ final: "t" }, (params) => {
    if (params[0] !== 8) return false;
    const rows = Number(params[1]);
    const cols = Number(params[2]);
    if (follows() && rows > 0 && cols > 0) {
      if (term.cols !== cols || term.rows !== rows) term.resize(cols, rows);
      onGrid(cols, rows);
    }
    return true;
  });
}
