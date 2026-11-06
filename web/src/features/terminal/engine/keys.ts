// Keys as the pane gets them: a one-shot Ctrl or Alt from the key bar applied to what was typed.

import type { Modifier } from "./types";

export const MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

// applyModifier encodes a printable character or an arrow under a one-shot Ctrl or Alt (an arrow
// as the modified CSI form, ESC [ 1 ; 5 A for Ctrl, ; 3 for Alt); anything else passes through.
export function applyModifier(mod: Modifier, seq: string): string {
  if (!mod) return seq;
  const arrow = /^\x1b[[O]([ABCD])$/.exec(seq);
  if (arrow) return `\x1b[1;${mod === "ctrl" ? 5 : 3}${arrow[1]}`;
  if (seq.length !== 1 || seq < " " || seq > "~") return seq;
  return mod === "ctrl" ? String.fromCharCode(seq.toUpperCase().charCodeAt(0) & 0x1f) : "\x1b" + seq;
}

// isFindKey is the find bar's key in the terminal lens — ⌘F on a Mac, Ctrl+Shift+F elsewhere, where Ctrl+F is the
// shell's own — which xterm must leave to the page.
export function isFindKey(e: { code: string; metaKey: boolean; ctrlKey: boolean; shiftKey: boolean; altKey: boolean }, mac: boolean): boolean {
  if (e.code !== "KeyF" || e.altKey) return false;
  return mac ? e.metaKey && !e.shiftKey : e.ctrlKey && e.shiftKey;
}

// bracketed wraps a pasted text the way xterm would with DECSET 2004 on: CRLF/LF → CR between the markers.
export const bracketed = (text: string) => "\x1b[200~" + text.replace(/\r\n/g, "\r").replace(/\n/g, "\r") + "\x1b[201~";
