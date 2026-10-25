// parseAnsi reads a terminal screen as herdr prints it — text with SGR colour sequences — into lines of spans, each
// with its colours and weight resolved, every other escape (cursor moves, erases, OSC titles) dropped. The wall's
// screen view draws the spans; nothing here touches the DOM.

import type { AnsiPalette } from "@/lib/theme";

export type Span = {
  text: string;
  fg?: string; // a CSS colour; absent for the default
  bg?: string;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
  strike?: boolean;
};
export type Line = Span[];

// xterm.js's own palette, the fallback; a screen tile draws with the theme's (lib/theme.ts ansiPalette), as the lens does
export const XTERM_PALETTE: AnsiPalette = {
  colors: [
    "#2e3436", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
    "#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec",
  ],
  fg: "#ffffff",
  bg: "#14120f",
};
export const DEFAULT_FG = XTERM_PALETTE.fg;
export const DEFAULT_BG = XTERM_PALETTE.bg;

type Colour = { index: number } | { css: string } | null;
type Style = { fg: Colour; bg: Colour; bold: boolean; dim: boolean; italic: boolean; underline: boolean; inverse: boolean; strike: boolean };

const plain = (): Style => ({ fg: null, bg: null, bold: false, dim: false, italic: false, underline: false, inverse: false, strike: false });

export function parseAnsi(text: string, palette: AnsiPalette = XTERM_PALETTE): Line[] {
  const lines: Line[] = [];
  let line: Line = [];
  let style = plain();
  let buffer = "";
  const flush = () => {
    if (buffer) line.push(span(buffer, style, palette));
    buffer = "";
  };
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]!;
    if (ch === "\x1b") {
      const next = text[i + 1];
      if (next === "[") {
        // CSI: parameter and intermediate bytes, then a final byte; only SGR (m) means anything on a screen dump
        let j = i + 2;
        while (j < text.length && text.charCodeAt(j) >= 0x20 && text.charCodeAt(j) <= 0x3f) j++;
        if (text[j] === "m") {
          flush();
          style = sgr(style, text.slice(i + 2, j));
        }
        i = j;
      } else if (next === "]") {
        // OSC, up to BEL or ESC \
        const bell = text.indexOf("\x07", i + 2);
        const st = text.indexOf("\x1b\\", i + 2);
        if (bell < 0 && st < 0) i = text.length;
        else if (st >= 0 && (bell < 0 || st < bell)) i = st + 1;
        else i = bell;
      } else if (next !== undefined && "()*+#%".includes(next)) {
        i += 2; // a charset designation carries one more byte
      } else {
        i += 1;
      }
      continue;
    }
    if (ch === "\n") {
      flush();
      lines.push(line);
      line = [];
      continue;
    }
    if (ch < " " && ch !== "\t") continue; // \r before \n, BEL, and the rest
    buffer += ch;
  }
  flush();
  lines.push(line);
  return lines;
}

// sgr applies one SGR sequence's parameters to a style
function sgr(style: Style, params: string): Style {
  const next = { ...style };
  const tokens = params === "" ? ["0"] : params.split(";");
  for (let k = 0; k < tokens.length; k++) {
    const sub = tokens[k]!.split(":").map((part) => (part === "" ? 0 : Number(part)));
    const code = sub[0]!;
    if (Number.isNaN(code)) continue;
    switch (code) {
      case 0:
        Object.assign(next, plain());
        break;
      case 1:
        next.bold = true;
        break;
      case 2:
        next.dim = true;
        break;
      case 3:
        next.italic = true;
        break;
      case 4:
        next.underline = true;
        break;
      case 7:
        next.inverse = true;
        break;
      case 9:
        next.strike = true;
        break;
      case 22:
        next.bold = false;
        next.dim = false;
        break;
      case 23:
        next.italic = false;
        break;
      case 24:
        next.underline = false;
        break;
      case 27:
        next.inverse = false;
        break;
      case 29:
        next.strike = false;
        break;
      case 39:
        next.fg = null;
        break;
      case 49:
        next.bg = null;
        break;
      case 38:
      case 48: {
        // 38;5;n or 38;2;r;g;b as separate tokens, or 38:5:n / 38:2::r:g:b (a colour space first) in one
        const colon = sub.length > 1;
        const args = colon ? sub.slice(1) : tokens.slice(k + 1).map(Number);
        let value: Colour = null;
        let used = 0;
        if (args[0] === 5 && args.length >= 2) {
          value = colour256(args[1]!);
          used = 2;
        } else if (args[0] === 2) {
          const rgb = colon && args.length >= 5 ? args.slice(2, 5) : args.slice(1, 4);
          if (rgb.length === 3 && rgb.every((v) => v >= 0 && v <= 255)) value = { css: hex(rgb) };
          used = 4;
        }
        if (!colon) k += used;
        if (code === 38) next.fg = value;
        else next.bg = value;
        break;
      }
      default:
        if (code >= 30 && code <= 37) next.fg = { index: code - 30 };
        else if (code >= 40 && code <= 47) next.bg = { index: code - 40 };
        else if (code >= 90 && code <= 97) next.fg = { index: code - 90 + 8 };
        else if (code >= 100 && code <= 107) next.bg = { index: code - 100 + 8 };
    }
  }
  return next;
}

// colour256 is one of the 256-colour table: the palette, a 6×6×6 cube, then 24 greys
function colour256(n: number): Colour {
  if (!(n >= 0 && n <= 255)) return null;
  if (n < 16) return { index: n };
  if (n < 232) {
    const v = n - 16;
    return { css: hex([Math.floor(v / 36), Math.floor(v / 6) % 6, v % 6].map((c) => (c ? 55 + 40 * c : 0))) };
  }
  const grey = 8 + 10 * (n - 232);
  return { css: hex([grey, grey, grey]) };
}

const hex = (rgb: number[]) => "#" + rgb.map((v) => v.toString(16).padStart(2, "0")).join("");

const css = (colour: Exclude<Colour, null>, palette: AnsiPalette) => ("css" in colour ? colour.css : palette.colors[colour.index]!);

// span resolves a style onto a run of text: bold brightens a basic colour as xterm draws it, inverse swaps the two
function span(text: string, style: Style, palette: AnsiPalette): Span {
  const out: Span = { text };
  let fg = style.fg;
  let bg = style.bg;
  if (style.bold && fg && "index" in fg && fg.index < 8) fg = { index: fg.index + 8 };
  if (style.inverse) [fg, bg] = [bg ?? { css: palette.bg }, fg ?? { css: palette.fg }];
  if (fg) out.fg = css(fg, palette);
  if (bg) out.bg = css(bg, palette);
  if (style.bold) out.bold = true;
  if (style.dim) out.dim = true;
  if (style.italic) out.italic = true;
  if (style.underline) out.underline = true;
  if (style.strike) out.strike = true;
  return out;
}
