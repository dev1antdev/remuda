// The screen's rows with their colours — what a timelapse draws, and the find's marks over them.

import { useLayoutEffect, useState, type RefObject } from "react";
import { parseAnsi, type Line, type Span } from "./ansi";
import { thumbFont } from "./engine/geometry";

const BASE_FONT = 13;

// ScreenLines draws the rows; current is the row a search stepped to, drawn apart
export function ScreenLines({ lines, current = -1 }: { lines: Line[]; current?: number }) {
  return lines.map((line, i) => (
    <div key={i} className={i === current ? "screen-line is-current" : "screen-line"}>
      {line.map((span, j) => (
        <span key={j} className={classOf(span)} style={styleOf(span)}>
          {span.text}
        </span>
      ))}
    </div>
  ));
}

const classOf = (span: Span) => [span.bold && "b", span.dim && "d", span.italic && "i", span.underline && "u", span.strike && "s", span.mark && "m"].filter(Boolean).join(" ") || undefined;

// styleOf is a span's own colours; a marked one wears the mark's, as the find bar's matches do
const styleOf = (span: Span) => (!span.mark && (span.fg || span.bg) ? { color: span.fg, background: span.bg } : undefined);

// widthOf is the columns a screen is drawn at: the pane's grid, or the longest row when there is none or a row runs past it
export const widthOf = (cols: number, lines: Line[]) => Math.max(cols, 1, ...lines.map((line) => line.reduce((n, span) => n + span.text.length, 0)));

// FitRule is the font size for cols columns in avail pixels, given a cell's width at the base size
export type FitRule = (avail: number, cols: number, cellWidth: number, baseSize: number) => number;

// useFitFont is the font size at which cols columns fill the box's width, measured in the box's own document and
// followed as the box resizes; the screen terminal fits by a rule of its own
export function useFitFont(box: RefObject<HTMLDivElement | null>, cols: number, fit: FitRule = thumbFont): number {
  const [avail, setAvail] = useState(0);
  const [cell, setCell] = useState(0);
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const doc = el.ownerDocument;
    const win = doc.defaultView ?? window;
    const probe = doc.createElement("span");
    probe.textContent = "0".repeat(20);
    probe.style.cssText = `position:absolute;visibility:hidden;white-space:pre;font:${BASE_FONT}px/1 var(--mono)`;
    el.appendChild(probe);
    setCell(probe.getBoundingClientRect().width / 20);
    probe.remove();
    const measure = () => {
      const style = win.getComputedStyle(el);
      setAvail(el.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight));
    };
    const observer = new win.ResizeObserver(measure);
    observer.observe(el);
    measure();
    return () => observer.disconnect();
  }, [box]);
  return avail && cell ? fit(avail, cols, cell, BASE_FONT) : BASE_FONT;
}
