import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CLOSE_AFTER, MOVED_WITHIN, OPEN_AFTER, hoverIntent, type Timers } from "./hoverIntent";

let moved = -Infinity;
const timers: Timers = {
  now: () => Date.now(),
  movedAt: () => moved,
  after: (ms, run) => setTimeout(run, ms),
  cancel: (timer) => clearTimeout(timer as ReturnType<typeof setTimeout>),
};

function card() {
  const opened: boolean[] = [];
  const intent = hoverIntent(timers, (open) => opened.push(open));
  return { intent, opened, last: () => opened[opened.length - 1] ?? false };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(1_000_000);
  moved = -Infinity;
});
afterEach(() => vi.useRealTimers());

describe("the hover card's timing", () => {
  it("opens once the pointer has rested on the row, not before", () => {
    const { intent, last } = card();
    moved = Date.now();
    intent.enter("mouse");
    vi.advanceTimersByTime(OPEN_AFTER - 1);
    expect(last()).toBe(false);
    vi.advanceTimersByTime(1);
    expect(last()).toBe(true);
  });

  it("opens nothing for a row a scroll slid under a still pointer, until the pointer moves", () => {
    const { intent, last } = card();
    moved = Date.now() - MOVED_WITHIN - 10;
    intent.enter("mouse");
    vi.advanceTimersByTime(OPEN_AFTER * 3);
    expect(last()).toBe(false);
    intent.move("mouse");
    vi.advanceTimersByTime(OPEN_AFTER);
    expect(last()).toBe(true);
  });

  it("closes a beat after the pointer left the row and the card both, and stays when it came back in time", () => {
    const { intent, last } = card();
    intent.move("mouse");
    vi.advanceTimersByTime(OPEN_AFTER);
    intent.leave("mouse");
    vi.advanceTimersByTime(CLOSE_AFTER - 50);
    intent.enter("mouse"); // crossed the gap onto the card
    vi.advanceTimersByTime(CLOSE_AFTER * 2);
    expect(last()).toBe(true);
    intent.leave("mouse");
    vi.advanceTimersByTime(CLOSE_AFTER);
    expect(last()).toBe(false);
  });

  it("never opens for a pointer that left before the rest was over, nor for a press", () => {
    const left = card();
    left.intent.move("mouse");
    vi.advanceTimersByTime(OPEN_AFTER - 100);
    left.intent.leave("mouse");
    vi.advanceTimersByTime(OPEN_AFTER);
    expect(left.opened).toEqual([]);

    const pressed = card();
    pressed.intent.move("mouse");
    pressed.intent.press();
    vi.advanceTimersByTime(OPEN_AFTER * 2);
    expect(pressed.opened).toEqual([]);
  });

  it("is deaf to a touch, which has no hover", () => {
    const { intent, opened } = card();
    moved = Date.now();
    intent.enter("touch");
    intent.move("touch");
    vi.advanceTimersByTime(OPEN_AFTER * 2);
    expect(opened).toEqual([]);
  });

  it("closes at once when told, and a closed card's timers fire nothing", () => {
    const { intent, opened } = card();
    intent.move("mouse");
    vi.advanceTimersByTime(OPEN_AFTER);
    intent.close();
    expect(opened).toEqual([true, false]);
    intent.move("mouse");
    intent.dispose();
    vi.advanceTimersByTime(OPEN_AFTER * 2);
    expect(opened).toEqual([true, false]);
  });
});
