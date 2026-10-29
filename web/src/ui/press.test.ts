import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { longPress, PRESS_DELAY } from "./press";

const at = (x: number, y: number, pointerType = "touch") => ({ pointerType, clientX: x, clientY: y });
const click = () => ({
  prevented: false,
  stopped: false,
  preventDefault() {
    this.prevented = true;
  },
  stopPropagation() {
    this.stopped = true;
  },
});

describe("longPress", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("fires after the delay held still and swallows the click that follows", () => {
    let pressed = 0;
    const h = longPress(() => pressed++);
    h.onPointerDown(at(10, 10));
    vi.advanceTimersByTime(PRESS_DELAY - 1);
    expect(pressed).toBe(0);
    vi.advanceTimersByTime(1);
    expect(pressed).toBe(1);
    h.onPointerUp();
    const c = click();
    h.onClickCapture(c);
    expect(c.prevented && c.stopped).toBe(true);
    const next = click();
    h.onClickCapture(next);
    expect(next.prevented).toBe(false);
  });

  it("is called off by a move, a lift or a cancel, and never by a mouse", () => {
    let pressed = 0;
    const h = longPress(() => pressed++);
    h.onPointerDown(at(10, 10));
    h.onPointerMove(at(12, 30));
    vi.advanceTimersByTime(PRESS_DELAY);
    expect(pressed).toBe(0);
    h.onPointerDown(at(10, 10));
    h.onPointerMove(at(14, 14));
    h.onPointerUp();
    vi.advanceTimersByTime(PRESS_DELAY);
    expect(pressed).toBe(0);
    h.onPointerDown(at(10, 10));
    h.onPointerCancel();
    vi.advanceTimersByTime(PRESS_DELAY);
    expect(pressed).toBe(0);
    h.onPointerDown(at(10, 10, "mouse"));
    vi.advanceTimersByTime(PRESS_DELAY);
    expect(pressed).toBe(0);
    const c = click();
    h.onClickCapture(c);
    expect(c.prevented).toBe(false);
  });

  it("keeps the context menu away while a finger is down", () => {
    const h = longPress(() => {});
    const menu = {
      prevented: false,
      preventDefault() {
        this.prevented = true;
      },
    };
    h.onContextMenu(menu);
    expect(menu.prevented).toBe(false);
    h.onPointerDown(at(0, 0));
    h.onContextMenu(menu);
    expect(menu.prevented).toBe(true);
  });
});
