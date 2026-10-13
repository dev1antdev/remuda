// herdr enables mouse tracking in every attach stream; honoured by xterm, every drag would be mouse reports.
// The tracking DECSETs are intercepted so xterm never tracks — the mouse stays ours and what the app asked
// for is synthesized by hand (mouse.ts) — and the modes that matter later are noted on the way.

import type { Terminal } from "@xterm/xterm";

export const TRACKING = new Set([9, 1000, 1002, 1003]);
const DECSET = /\x1b\[\?([\d;]+)([hl])/g;

// splitTracking splits a DECSET that mixes a mouse-tracking mode with other modes in two, the others
// first: a sequence re-injected from xterm's CSI handler would land behind the rest of the chunk.
export function splitTracking(s: string): string {
  if (!s.includes("\x1b[?")) return s;
  return s.replace(DECSET, (all, params: string, final: string) => {
    if (!params.includes(";")) return all;
    const ns = params.split(";");
    const ours = ns.filter((n) => TRACKING.has(Number(n)));
    if (ours.length === 0 || ours.length === ns.length) return all;
    const theirs = ns.filter((n) => !TRACKING.has(Number(n)));
    return `\x1b[?${theirs.join(";")}${final}\x1b[?${ours.join(";")}${final}`;
  });
}

export type Modes = {
  appMouse: () => number; // the tracking mode the app wants; 0 = none
  sgrMouse: () => boolean; // it asked for SGR (1006) reports
  pasteModeSeen: () => boolean; // a DECSET 2004 passed: a late attach gets no replay, so Claude Code's "bracketed paste on" is never seen
  forgetPaste: () => void;
  dispose: () => void;
};

export function interceptModes(term: Terminal, mount: HTMLElement): Modes {
  let appMouse = 0;
  let sgrMouse = false;
  let pasteModeSeen = false;
  const handler = (enable: boolean) => (params: (number | number[])[]) => {
    const rest: number[] = [];
    let touched = false;
    for (const p of params) {
      const n = Array.isArray(p) ? p[0]! : p;
      if (TRACKING.has(n)) {
        touched = true;
        appMouse = enable ? n : 0;
      } else {
        if (n === 1006) sgrMouse = enable;
        if (n === 2004) pasteModeSeen = true;
        rest.push(n);
      }
    }
    if (!touched) return false; // nothing of ours: xterm handles it
    // a mixed sequence only gets here when a chunk boundary cut it before splitTracking could: the rest is re-applied, late
    if (rest.length) term.write(`\x1b[?${rest.join(";")}${enable ? "h" : "l"}`);
    // xterm shows an arrow cursor only when it tracks the mouse itself; the app owns the clicks, so the arrow is ours
    mount.classList.toggle("app-mouse", appMouse !== 0);
    return true;
  };
  const decset = term.parser.registerCsiHandler({ prefix: "?", final: "h" }, handler(true));
  const decrst = term.parser.registerCsiHandler({ prefix: "?", final: "l" }, handler(false));
  return {
    appMouse: () => appMouse,
    sgrMouse: () => sgrMouse,
    pasteModeSeen: () => pasteModeSeen,
    forgetPaste: () => {
      pasteModeSeen = false;
    },
    dispose: () => {
      decset.dispose();
      decrst.dispose();
    },
  };
}
