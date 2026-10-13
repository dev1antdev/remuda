// The pointer on the grid: the wheel and a finger become the app's scroll reports, a press and release
// within 4px its click (after xterm's own mouseup, so a click that opened a link is not also the app's),
// and a drag is a selection that never reaches the app.

import type { Terminal } from "@xterm/xterm";
import { cellSize } from "./geometry";
import type { Modes } from "./modes";
import { modifierBits, type Reporter } from "./mouse";
import type { Selection } from "./selection";

export type Flags = { linkUsed: boolean }; // a link took this click

export function installPointer(term: Terminal, mount: HTMLElement, { modes, reporter, selection, layer, flags }: { modes: Modes; reporter: Reporter; selection: Selection; layer: HTMLElement; flags: Flags }): () => void {
  let pressed: { x: number; y: number; button: number } | null = null;

  const onWheel = (e: WheelEvent) => {
    const sideways = Math.abs(e.deltaX) > Math.abs(e.deltaY);
    // a grid larger than the screen scrolls here in that direction; otherwise the gesture is the app's (wheel left/right = buttons 6/7)
    if (sideways ? mount.scrollWidth > mount.clientWidth : mount.classList.contains("keep") && mount.scrollHeight > mount.clientHeight) {
      e.stopPropagation();
      return;
    }
    if (!modes.appMouse()) return;
    const cell = reporter.cellAt(e);
    if (!cell) return;
    e.preventDefault();
    e.stopPropagation();
    if (pressed) selection.pointerAt(e); // a wheel under a held button
    reporter.scroll(reporter.wheel(e, sideways), sideways, cell, modifierBits(e));
  };

  // A finger drag is the app's scroll too, one tick per row moved: otherwise xterm would scroll its
  // own (empty) buffer and the app would never know. A tap stays a tap, reported as a click below.
  let touch: { y: number; x: number; acc: number } | null = null;
  const onTouchStart = (e: TouchEvent) => {
    touch = null;
    if (!modes.appMouse() || !layer.hidden || e.touches.length !== 1) return;
    const t = e.touches[0]!;
    touch = { x: t.clientX, y: t.clientY, acc: 0 };
  };
  const onTouchMove = (e: TouchEvent) => {
    if (!touch || e.touches.length !== 1) return;
    const t = e.touches[0]!;
    const dy = touch.y - t.clientY; // the finger goes up: the text follows it, the view scrolls down
    touch.y = t.clientY;
    touch.acc += dy;
    const h = cellSize(term).h;
    const ticks = Math.trunc(touch.acc / h);
    touch.acc -= ticks * h;
    e.preventDefault();
    e.stopPropagation();
    const cell = reporter.cellAt(t);
    if (cell) reporter.scroll(ticks, false, cell);
  };
  const onTouchEnd = () => {
    touch = null;
  };

  const onMouseDown = (e: MouseEvent) => {
    pressed = { x: e.clientX, y: e.clientY, button: e.button };
    flags.linkUsed = false;
    if (e.button === 0 && !e.shiftKey && !layer.contains(e.target as Node)) selection.dragStart(e);
  };
  const onMouseUp = (e: MouseEvent) => {
    const p = pressed;
    pressed = null;
    selection.dragEnd();
    if (!p || !modes.appMouse() || layer.contains(e.target as Node)) return;
    if (p.button !== e.button || Math.hypot(e.clientX - p.x, e.clientY - p.y) > 4) return;
    const cell = reporter.cellAt(e);
    if (!cell) return;
    const code = e.button + modifierBits(e);
    setTimeout(() => {
      if (!flags.linkUsed) reporter.click(code, cell);
    }, 0);
  };

  mount.addEventListener("wheel", onWheel, { capture: true, passive: false });
  mount.addEventListener("touchstart", onTouchStart, { capture: true, passive: true });
  mount.addEventListener("touchmove", onTouchMove, { capture: true, passive: false });
  mount.addEventListener("touchend", onTouchEnd, { capture: true, passive: true });
  mount.addEventListener("touchcancel", onTouchEnd, { capture: true, passive: true });
  mount.addEventListener("mousedown", onMouseDown, true);
  mount.addEventListener("mouseup", onMouseUp, true);
  return () => {
    mount.removeEventListener("wheel", onWheel, { capture: true });
    mount.removeEventListener("touchstart", onTouchStart, { capture: true });
    mount.removeEventListener("touchmove", onTouchMove, { capture: true });
    mount.removeEventListener("touchend", onTouchEnd, { capture: true });
    mount.removeEventListener("touchcancel", onTouchEnd, { capture: true });
    mount.removeEventListener("mousedown", onMouseDown, true);
    mount.removeEventListener("mouseup", onMouseUp, true);
  };
}
