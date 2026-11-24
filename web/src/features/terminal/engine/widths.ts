// The page's cell widths: the core's own. libghostty clusters graphemes (mode 2027 on) and counts an emoji with a
// variation selector as two cells — ✍️ is wide there — while xterm's Unicode 11 tables keep it one. A joined row comes
// as one line for the viewer to wrap, so counting a cell differently from the core puts every cell after it on another
// column, and the row lies elsewhere than in the core (schema/README.md, "Attach streams").
//
// The tables are the addon's (`@xterm/addon-unicode11`, the same project as xterm); this adds the one rule they lack.

import { Unicode11Addon } from "@xterm/addon-unicode11";
import type { IUnicodeVersionProvider, Terminal } from "@xterm/xterm";

const VARIATION16 = 0xfe0f;

// xterm packs a cell's width and whether it joins the cell before into one number (UnicodeService's
// createPropertyValue): the state, then the width in bits 1-2, then the join in bit 0.
const widthOf = (properties: number): number => (properties >> 1) & 0b11;
const twoCellsJoined = (2 << 1) | 1;

class CoreWidths implements IUnicodeVersionProvider {
  constructor(private readonly table: IUnicodeVersionProvider) {}

  get version(): string {
    return this.table.version;
  }

  wcwidth(codepoint: number): 0 | 1 | 2 {
    return this.table.wcwidth(codepoint);
  }

  charProperties(codepoint: number, preceding: number): number {
    // A variation selector after a one-cell character: the emoji presentation, which is two cells wide as in the core.
    if (codepoint === VARIATION16 && widthOf(preceding) === 1) {
      return twoCellsJoined;
    }
    return this.table.charProperties(codepoint, preceding);
  }
}

// installWidths gives term the core's cell widths: Unicode 11's tables and the variation-selector rule above.
export function installWidths(term: Terminal): void {
  // The addon hands its provider to whatever terminal activates it; this one only takes it, so it can be wrapped.
  let table: IUnicodeVersionProvider | undefined;
  const register = (provider: IUnicodeVersionProvider) => (table = provider);
  const shim = { unicode: { register } } as unknown as Terminal;
  new Unicode11Addon().activate(shim);
  if (!table) {
    return; // an addon that registers nothing: the page keeps xterm's own widths rather than none
  }
  term.unicode.register(new CoreWidths(table));
  term.unicode.activeVersion = "11";
}
