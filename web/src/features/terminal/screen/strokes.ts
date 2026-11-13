// What the screen terminal sends: a keystroke as herdr names it, or text — typed, or a paste — and how strokes typed
// quickly are coalesced into one request, text first, then the keys after it. Pure.

import type { Modifier } from "../engine/types";

export type Stroke = { text: string; paste?: boolean } | { key: string };

// Batch is one request of POST /api/input: the text, then the keys
export type Batch = { text: string; paste: boolean; keys: string[] };

// MAX_TEXT bounds the typed text one request carries; a paste goes whole (the server refuses one past 64 KB)
export const MAX_TEXT = 4096;
// MAX_KEYS bounds the keys one request carries
export const MAX_KEYS = 64;

export type KeyLike = { key: string; code: string; ctrlKey: boolean; altKey: boolean; shiftKey: boolean; metaKey: boolean; isComposing?: boolean };

// NAMED are the keys herdr names, by the browser's key
const NAMED: Record<string, string> = {
  Enter: "enter",
  Escape: "esc",
  Tab: "tab",
  Backspace: "backspace",
  ArrowUp: "up",
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
  Home: "home",
  End: "end",
  PageUp: "pageup",
  PageDown: "pagedown",
  Delete: "delete",
};

// UNMODIFIED are the keys the server sends as a terminal's bytes, which carry no modifier
const UNMODIFIED = new Set(["home", "end", "pageup", "pagedown", "delete", "tab"]);

const letterOf = (code: string) => /^Key([A-Z])$/.exec(code)?.[1]?.toLowerCase() ?? /^Digit([0-9])$/.exec(code)?.[1] ?? null;

// keyOf is the key a keydown presses in herdr's words, or null when it is no key of the pane's: a printable character
// (it arrives as text), a composition, ⌘ anything, Ctrl+V and Ctrl+Shift+… off a Mac (the browser's paste and shortcuts)
export function keyOf(e: KeyLike, mac: boolean): string | null {
  if (e.isComposing || e.key === "Process" || e.key === "Dead" || e.metaKey) return null;
  const named = NAMED[e.key];
  if (named) {
    if (named === "tab") return e.ctrlKey || e.altKey ? null : e.shiftKey ? "shift+tab" : "tab";
    if (UNMODIFIED.has(named)) return e.ctrlKey || e.altKey || e.shiftKey ? null : named;
    const mods = [e.ctrlKey && "ctrl", e.altKey && "alt", e.shiftKey && named !== "esc" && named !== "backspace" && "shift"].filter(Boolean);
    return [...mods, named].join("+");
  }
  const letter = letterOf(e.code);
  if (e.ctrlKey && !e.altKey && !e.shiftKey) {
    if (letter) return !mac && letter === "v" ? null : `ctrl+${letter}`;
    if (e.code === "BracketLeft") return "esc";
    if (e.code === "Space") return "ctrl+space";
  }
  if (e.altKey && !e.ctrlKey && !e.shiftKey && !mac && letter) return `alt+${letter}`; // a Mac's Option types a character
  return null;
}

// SEQUENCES are what the key bar types — a terminal's bytes — as herdr's keys
const SEQUENCES: Record<string, string> = {
  "\x1b": "esc",
  "\t": "tab",
  "\r": "enter",
  "\x1b\r": "alt+enter",
  "\x7f": "backspace",
  "\x1b[Z": "shift+tab",
  "\x1b[A": "up",
  "\x1b[B": "down",
  "\x1b[C": "right",
  "\x1b[D": "left",
  "\x1bOA": "up",
  "\x1bOB": "down",
  "\x1bOC": "right",
  "\x1bOD": "left",
  "\x1b[H": "home",
  "\x1b[F": "end",
  "\x1b[5~": "pageup",
  "\x1b[6~": "pagedown",
  "\x1b[3~": "delete",
};

// strokeOf is a key bar's sequence as a stroke: a key herdr names, a control character as Ctrl and its letter, else text
export function strokeOf(seq: string): Stroke {
  const named = SEQUENCES[seq];
  if (named) return { key: named };
  if (seq.length === 1) {
    const code = seq.charCodeAt(0);
    if (code >= 1 && code <= 26) return { key: `ctrl+${String.fromCharCode(code + 96)}` };
  }
  return { text: seq };
}

// withModifier applies the key bar's one-shot Ctrl or Alt to the next stroke: an arrow, Enter or Backspace, or a single
// letter or digit typed; anything else goes as it is, and used says whether the modifier was spent
export function withModifier(stroke: Stroke, mod: Modifier): { stroke: Stroke; used: boolean } {
  if (!mod) return { stroke, used: false };
  if ("key" in stroke) {
    if (stroke.key.includes("+") || UNMODIFIED.has(stroke.key)) return { stroke, used: false };
    return { stroke: { key: `${mod}+${stroke.key}` }, used: true };
  }
  const letter = stroke.text.length === 1 && !stroke.paste ? stroke.text.toLowerCase() : "";
  if (!/^[a-z0-9]$/.test(letter)) return { stroke, used: false };
  return { stroke: { key: `${mod}+${letter}` }, used: true };
}

// takeBatch takes the next request off the front of the queue: typed text, or one paste, then the keys after it; a text
// after a key waits for the next request, so the order typed is the order sent
export function takeBatch(queue: Stroke[]): { batch: Batch; rest: Stroke[] } | null {
  if (!queue.length) return null;
  let text = "";
  let paste = false;
  const keys: string[] = [];
  let i = 0;
  for (; i < queue.length; i++) {
    const stroke = queue[i]!;
    if ("key" in stroke) {
      if (keys.length >= MAX_KEYS) break;
      keys.push(stroke.key);
      continue;
    }
    if (i > 0 && (keys.length || paste || stroke.paste || text.length + stroke.text.length > MAX_TEXT)) break;
    paste = !!stroke.paste;
    text += stroke.text;
  }
  return { batch: { text, paste, keys }, rest: queue.slice(i) };
}
