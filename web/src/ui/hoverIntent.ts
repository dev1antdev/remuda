// When a hover card opens and closes, after Orca's: resting on its row for OPEN_AFTER opens it, leaving both the row and
// the card closes it CLOSE_AFTER later, and a row that slid under a still pointer — the list scrolled — opens nothing
// until the pointer itself moves. A touch never hovers. Pure but for the clock and timers it is handed, so a test
// drives it.

export const OPEN_AFTER = 450;
export const CLOSE_AFTER = 250;
// MOVED_WITHIN is how recent the pointer's own move must be for an entry to count as the pointer's, not the list's
export const MOVED_WITHIN = 150;

// Timers are the clock, the timers, and when the pointer last moved anywhere on the page
export type Timers = { now: () => number; movedAt: () => number; after: (ms: number, run: () => void) => unknown; cancel: (timer: unknown) => void };

export type HoverIntent = {
  enter: (pointerType: string) => void; // the pointer came over the row or its card
  move: (pointerType: string) => void; // the pointer moved over them
  leave: (pointerType: string) => void; // the pointer left both
  press: () => void; // a press on the row: a click is no rest
  close: () => void; // closed from outside: Esc, a press elsewhere
  dispose: () => void;
};

export function hoverIntent(timers: Timers, setOpen: (open: boolean) => void): HoverIntent {
  let open = false;
  let over = false;
  let opening: unknown = null;
  let closing: unknown = null;
  const stopOpening = () => {
    if (opening !== null) timers.cancel(opening);
    opening = null;
  };
  const stopClosing = () => {
    if (closing !== null) timers.cancel(closing);
    closing = null;
  };
  const show = (next: boolean) => {
    if (open === next) return;
    open = next;
    setOpen(next);
  };
  const schedule = () => {
    if (open || opening !== null) return;
    opening = timers.after(OPEN_AFTER, () => {
      opening = null;
      if (over) show(true);
    });
  };
  return {
    enter(pointerType) {
      if (pointerType === "touch") return;
      over = true;
      stopClosing();
      if (timers.now() - timers.movedAt() < MOVED_WITHIN) schedule();
    },
    move(pointerType) {
      if (pointerType === "touch") return;
      over = true;
      stopClosing();
      schedule();
    },
    leave(pointerType) {
      if (pointerType === "touch") return;
      over = false;
      stopOpening();
      if (open && closing === null)
        closing = timers.after(CLOSE_AFTER, () => {
          closing = null;
          if (!over) show(false);
        });
    },
    press() {
      stopOpening();
    },
    close() {
      stopOpening();
      stopClosing();
      show(false);
    },
    dispose() {
      stopOpening();
      stopClosing();
    },
  };
}
