// A panel's draggable edge: a thin handle beside the panel in the same row, dragged with the pointer or stepped with the
// keyboard, a double-click back to the usual width. The width is a pref per device and reaches the stylesheet as a CSS
// variable on the row's element, so a drag moves the layout and redraws nothing else; the pref is written when the drag ends.

import { useLayoutEffect, useRef, type PointerEvent } from "react";
import { usePref, writePref } from "@/lib/prefs";
import { clampWidth, dragWidth, keyWidth, PANELS, type Edge, type PanelSpec } from "./panels";
import { t } from "@/lib/i18n";

export default function Resizer({ spec, edge, label }: { spec: PanelSpec; edge: Edge; label: string }) {
  const [stored, setStored] = usePref<number>(spec.key, spec.width);
  const width = clampWidth(spec, stored);
  const handle = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; start: number; width: number } | null>(null);
  const paint = (w: number) => handle.current?.parentElement?.style.setProperty(spec.cssVar, `${w}px`);

  useLayoutEffect(() => {
    paint(width);
  }, [width, spec.cssVar]); // eslint-disable-line react-hooks/exhaustive-deps

  const begin = (e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    e.preventDefault();
    drag.current = { x: e.clientX, start: width, width };
    e.currentTarget.setPointerCapture(e.pointerId);
    document.body.classList.add("is-resizing");
  };
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d) return;
    d.width = dragWidth(spec, d.start, e.clientX - d.x, edge);
    paint(d.width);
  };
  const end = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d) return;
    drag.current = null;
    document.body.classList.remove("is-resizing");
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    if (d.width !== width) setStored(d.width);
    else paint(width);
  };

  return (
    <div
      ref={handle}
      className={`resizer resizer-${edge}`}
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={width}
      aria-valuemin={spec.min}
      aria-valuemax={spec.max}
      tabIndex={0}
      title={t("{label}: drag it, or ← → once it has the focus; double-click for the usual {px} px", { label, px: spec.width })}
      onPointerDown={begin}
      onPointerMove={move}
      onPointerUp={end}
      onPointerCancel={end}
      onDoubleClick={() => setStored(spec.width)}
      onKeyDown={(e) => {
        const next = keyWidth(spec, width, e.key, e.shiftKey, edge);
        if (next === null) return;
        e.preventDefault();
        setStored(next);
      }}
    />
  );
}

// resetPanels puts every panel back to its usual width on this device
export function resetPanels(): void {
  for (const spec of Object.values(PANELS)) writePref(spec.key, spec.width);
}
