// The selection-mode layer: a transparent text copy of the screen whose selection is the browser's own (a phone's long-press).

import type { Terminal } from "@xterm/xterm";
import { cellSize } from "./geometry";

export type Layer = {
  el: HTMLElement;
  show: () => void;
  hide: () => void;
  schedule: () => void; // the screen changed: repaint the copy soon
  dispose: () => void;
};

export function createLayer(term: Terminal): Layer {
  const el = document.createElement("div");
  el.className = "term-select";
  el.hidden = true;
  term.element!.appendChild(el);
  let timer: ReturnType<typeof setTimeout> | null = null;
  let held = false; // a selection in the copy stalled its repaint: see paint

  // selecting says a selection of the person's lies in the copy (a phone's long-press does it in the layer).
  const selecting = () => {
    const sel = window.getSelection();
    return !!sel && sel.rangeCount > 0 && !sel.isCollapsed && el.contains(sel.getRangeAt(0).commonAncestorContainer);
  };

  // paint mirrors the visible rows into the layer as text, one row per line. Replacing the text replaces the nodes the
  // browser's selection points at, so a repaint while the person selects (the spinner redraws every second) rubs the
  // selection out: it waits, and the next paint after the selection goes comes from selectionchange.
  const paint = () => {
    timer = null;
    if (el.hidden) return;
    if (selecting()) {
      held = true;
      return;
    }
    held = false;
    const screen = term.element!.querySelector(".xterm-screen") as HTMLElement | null;
    if (screen) {
      el.style.width = `${screen.offsetWidth}px`;
      el.style.height = `${screen.offsetHeight}px`;
    }
    el.style.fontSize = `${term.options.fontSize}px`;
    el.style.lineHeight = `${cellSize(term).h}px`;
    const buf = term.buffer.active;
    const lines: string[] = [];
    for (let y = 0; y < term.rows; y++) {
      lines.push(buf.getLine(buf.viewportY + y)?.translateToString(true) ?? "");
    }
    const text = lines.join("\n");
    if (el.textContent !== text) el.textContent = text;
  };
  const schedule = () => {
    if (!el.hidden && !timer) timer = setTimeout(paint, 80);
  };
  const onScroll = term.onScroll(schedule);
  const onSelectionChange = () => {
    if (held && !selecting()) schedule();
  };
  document.addEventListener("selectionchange", onSelectionChange);
  return {
    el,
    show: () => {
      el.hidden = false;
      paint();
    },
    hide: () => {
      el.hidden = true;
      window.getSelection()?.removeAllRanges();
    },
    schedule,
    dispose: () => {
      onScroll.dispose();
      document.removeEventListener("selectionchange", onSelectionChange);
      if (timer) clearTimeout(timer);
    },
  };
}
