// The GIF export's drawing: every frame's screen laid out in cells on a canvas, in the flavour's terminal colours, only
// the rows that moved drawn again and handed to the encoder as one band, each pixel taken as its cell's ink or ground so
// the palette holds exactly the colours the cells wear. Runs in the page, a few frames at a time, so it never stalls.

import { t } from "@/lib/i18n";
import type { AnsiPalette } from "@/lib/theme";
import { parseAnsi } from "../ansi";
import { cellsOf, rgbOf, rowKey, type Cell, type Defaults } from "./cells";
import { GifWriter } from "./gif";
import { indexer, isInk, Tally } from "./palette";
import { gifDelays, screenText, type ScreenRows, type Speed } from "./playback";
import { Slicer } from "./slice";

const FONT_PX = 12;
const LINE = 15;
const PAD = 8;
export const MAX_COLS = 400;
export const MAX_ROWS = 200;

export type GifSource = { screens: ScreenRows[]; times: number[]; cols: number };

// grid is a screen as rows of cells, as many rows as the canvas has
function grid(screen: ScreenRows, palette: AnsiPalette, defaults: Defaults, rows: number): Cell[][] {
  const lines = parseAnsi(screenText(screen), palette);
  return Array.from({ length: rows }, (_, r) => (r < lines.length ? cellsOf(lines[r]!, defaults) : []));
}

// pause gives the page its turn through a message to itself, never a timer, which a tab in the background holds back a
// second or more at a time: an export left drawing behind another tab would take hours
const pause = () =>
  new Promise<void>((resolve) => {
    const channel = new MessageChannel();
    channel.port1.onmessage = () => {
      channel.port1.close();
      resolve();
    };
    channel.port2.postMessage(null);
  });

// exportGif draws the recording as a looping GIF at the speed picked; onProgress hears each frame drawn
export async function exportGif(source: GifSource, palette: AnsiPalette, font: string, speed: Speed, onProgress: (done: number, total: number) => void, signal?: AbortSignal): Promise<Blob> {
  const { screens, times } = source;
  const defaults: Defaults = { fg: rgbOf(palette.fg, 0xffffff), bg: rgbOf(palette.bg, 0) };
  let rows = 1;
  let cols = Math.max(20, source.cols);
  const tally = new Tally();
  const slice = new Slicer(() => performance.now());
  for (let i = 0; i < screens.length; i++) {
    const lines = parseAnsi(screenText(screens[i]), palette);
    rows = Math.max(rows, lines.length);
    for (const line of lines) {
      const cells = cellsOf(line, defaults);
      cols = Math.max(cols, cells.length);
      for (const cell of cells) {
        tally.add(cell.bg);
        if (cell.ch.trim()) tally.add(cell.fg);
      }
    }
    if (slice.due) {
      await pause();
      slice.restart();
    }
  }
  rows = Math.min(rows, MAX_ROWS);
  cols = Math.min(cols, MAX_COLS);

  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) throw new Error(t("This browser cannot draw the GIF: it has no canvas"));
  ctx.font = `${FONT_PX}px ${font}`;
  const cellW = Math.ceil(ctx.measureText("M").width);
  canvas.width = PAD * 2 + cols * cellW;
  canvas.height = PAD * 2 + rows * LINE;
  const colours = tally.palette([defaults.bg, defaults.fg]);
  const indexOf = indexer(colours);
  const gif = new GifWriter(canvas.width, canvas.height, colours);
  const delays = gifDelays(times, speed);
  const hex = (rgb: number) => `#${rgb.toString(16).padStart(6, "0")}`;

  let previous: string[] = [];
  let pending: Parameters<GifWriter["add"]>[0] | null = null;
  for (let i = 0; i < screens.length; i++) {
    if (signal?.aborted) throw new DOMException("the export was called off", "AbortError");
    const cells = grid(screens[i]!, palette, defaults, rows);
    const keys = cells.map(rowKey);
    const moved = keys.flatMap((key, r) => (i === 0 || key !== previous[r] ? [r] : []));
    previous = keys;
    if (moved.length === 0 && pending) {
      pending.delay += delays[i]!;
      continue;
    }
    const first = i === 0 ? 0 : moved[0]!;
    const last = i === 0 ? rows - 1 : moved[moved.length - 1]!;
    const top = i === 0 ? 0 : PAD + first * LINE;
    const bottom = i === 0 ? canvas.height : PAD + (last + 1) * LINE;
    ctx.fillStyle = hex(defaults.bg);
    ctx.fillRect(0, top, canvas.width, bottom - top);
    for (let r = first; r <= last; r++) drawRow(ctx, cells[r]!, PAD + r * LINE, cellW, defaults, font, hex);
    const band = ctx.getImageData(0, top, canvas.width, bottom - top);
    if (pending) gif.add(pending);
    pending = { x: 0, y: top, width: canvas.width, height: bottom - top, indices: quantize(band, top, cells, cellW, defaults, indexOf), delay: delays[i]! };
    if (slice.due) {
      onProgress(i + 1, screens.length);
      await pause();
      slice.restart();
    }
  }
  if (pending) gif.add(pending);
  onProgress(screens.length, screens.length);
  return new Blob([gif.finish() as BlobPart], { type: "image/gif" });
}

// drawRow paints one row: the grounds that are not the screen's own, then each character in its cell
function drawRow(ctx: CanvasRenderingContext2D, row: Cell[], y: number, cellW: number, defaults: Defaults, font: string, hex: (rgb: number) => string) {
  row.forEach((cell, c) => {
    if (cell.bg === defaults.bg) return;
    ctx.fillStyle = hex(cell.bg);
    ctx.fillRect(PAD + c * cellW, y, cellW, LINE);
  });
  ctx.textBaseline = "middle";
  let face = "";
  row.forEach((cell, c) => {
    const x = PAD + c * cellW;
    ctx.fillStyle = hex(cell.fg);
    if (cell.ch.trim()) {
      const wanted = `${cell.italic ? "italic " : ""}${cell.bold ? "bold " : ""}${FONT_PX}px ${font}`;
      if (wanted !== face) ctx.font = face = wanted;
      ctx.fillText(cell.ch, x, y + LINE / 2 + 0.5);
    }
    if (cell.underline) ctx.fillRect(x, y + LINE - 2, cellW, 1);
    if (cell.strike) ctx.fillRect(x, y + Math.round(LINE / 2), cellW, 1);
  });
}

// quantize turns a band of the canvas into palette indices, each pixel its cell's ink or its ground
function quantize(band: ImageData, top: number, cells: Cell[][], cellW: number, defaults: Defaults, indexOf: (rgb: number) => number): Uint8Array {
  const { width, height, data } = band;
  const out = new Uint8Array(width * height);
  const groundIndex = indexOf(defaults.bg);
  const inkIndex = indexOf(defaults.fg);
  for (let y = 0; y < height; y++) {
    const r = Math.floor((top + y - PAD) / LINE);
    const row = r >= 0 && r < cells.length ? cells[r]! : [];
    const inks = row.map((cell) => indexOf(cell.fg));
    const grounds = row.map((cell) => indexOf(cell.bg));
    for (let x = 0; x < width; x++) {
      const c = Math.floor((x - PAD) / cellW);
      const cell = x >= PAD && c < row.length ? row[c] : undefined;
      const at = (y * width + x) * 4;
      const ink = cell?.fg ?? defaults.fg;
      const ground = cell?.bg ?? defaults.bg;
      const inside = top + y >= PAD && top + y < PAD + cells.length * LINE && x >= PAD;
      out[y * width + x] = inside && isInk(data[at]!, data[at + 1]!, data[at + 2]!, ink, ground) ? (cell ? inks[c]! : inkIndex) : cell ? grounds[c]! : groundIndex;
    }
  }
  return out;
}
