// Screen is a pane's terminal as it is — the visible rows with their colours, read from herdr at the tiles' pace —
// drawn at the size that fits the width it is given, the bottom rows kept when the height runs out (the prompt and
// the status live there). The wall's screen view; a click opens the pane's terminal in Agents.

import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react";
import { useFlavor } from "@/app/theme";
import { useQuery } from "@/lib/query";
import { ansiPalette } from "@/lib/theme";
import { Notice } from "@/ui";
import { parseAnsi, type Span } from "./ansi";
import { fetchScreen } from "./api";
import { thumbFont } from "./engine/geometry";
import { t } from "@/lib/i18n";

const BASE_FONT = 13;

export const Screen = memo(function Screen({ machineId, paneId, cols, busy, active, onOpen }: {
  machineId: string;
  paneId: string;
  cols: number; // the pane's own grid, 0 when unknown
  busy: boolean; // the agent works or asks: the faster pace
  active: boolean; // on screen: a hidden screen keeps its rows and waits
  onOpen: () => void;
}) {
  const etag = useRef<string | null>(null);
  useEffect(() => {
    etag.current = null;
  }, [machineId, paneId]);
  const { data: text, error } = useQuery<string>(
    async () => {
      const res = await fetchScreen(machineId, paneId, etag.current);
      if (!res) return undefined;
      etag.current = res.etag;
      return res.text;
    },
    [machineId, paneId],
    { every: busy ? 3000 : 15000, enabled: active },
  );
  const flavor = useFlavor();
  const palette = useMemo(() => ansiPalette(flavor), [flavor]);
  const lines = useMemo(() => (text === null ? [] : parseAnsi(text, palette)), [text, palette]);
  // the pane's grid, or the longest row when the roster has none or a row runs past it (a remote's grid can lag)
  const width = Math.max(cols, 1, ...lines.map((line) => line.reduce((n, span) => n + span.text.length, 0)));
  const box = useRef<HTMLDivElement>(null);
  const fontSize = useFitFont(box, width);
  return (
    <div className="screen" ref={box} style={{ fontSize }} onClick={onOpen} title={t("Open the terminal in Agents")}>
      {text === null && !error && <div className="empty chat-empty">{t("Loading…")}</div>}
      {error && (
        <Notice tone="error" className="chat-notice">
          {error}
        </Notice>
      )}
      {lines.map((line, i) => (
        <div key={i} className="screen-line">
          {line.map((span, j) => (
            <span key={j} className={classOf(span)} style={span.fg || span.bg ? { color: span.fg, background: span.bg } : undefined}>
              {span.text}
            </span>
          ))}
        </div>
      ))}
    </div>
  );
});

const classOf = (span: Span) => [span.bold && "b", span.dim && "d", span.italic && "i", span.underline && "u", span.strike && "s"].filter(Boolean).join(" ") || undefined;

// useFitFont is the font size at which cols columns fill the box's width, measured in the box's own document (the
// pop-out has one of its own) and followed as the box resizes
function useFitFont(box: RefObject<HTMLDivElement | null>, cols: number): number {
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
  return avail && cell ? thumbFont(avail, cols, cell, BASE_FONT) : BASE_FONT;
}
