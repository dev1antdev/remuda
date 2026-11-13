// A card beside a row the pointer rests on: Radix's HoverCard in the design system's clothes, its timing ours
// (hoverIntent.ts) — open after a rest, closed a beat after the pointer left the row and the card both, nothing from a
// list scrolling under a still pointer. It never takes the keyboard: its buttons are the pointer's. On a touch screen
// it opens only when asked (pin), and a tap outside or Esc closes it.

import { HoverCard as Card } from "radix-ui";
import { useEffect, useMemo, useRef, useState, type PointerEvent, type ReactNode } from "react";
import { hoverIntent, type Timers } from "./hoverIntent";
import { usePortalHost } from "./portal";

// movedAt is when the pointer last moved anywhere on the page, heard once for every card
let movedAt = -Infinity;
let listening = false;
function hearMoves() {
  if (listening || typeof window === "undefined") return;
  listening = true;
  window.addEventListener("pointermove", (e) => e.pointerType !== "touch" && (movedAt = performance.now()), { capture: true, passive: true });
}

const TIMERS: Timers = {
  now: () => performance.now(),
  movedAt: () => movedAt,
  after: (ms, run) => window.setTimeout(run, ms),
  cancel: (timer) => window.clearTimeout(timer as number),
};

type TriggerProps = {
  onPointerEnter?: (e: PointerEvent) => void;
  onPointerMove?: (e: PointerEvent) => void;
  onPointerLeave?: (e: PointerEvent) => void;
  onPointerDown?: (e: PointerEvent) => void;
};

// useHoverCard is a row's card: whether it is open, the handlers the row spreads, and pin to open or close it by hand.
// The row's React subtree holds the card's portal, so its enter and leave cover the row and the card both.
export function useHoverCard(enabled: boolean): { open: boolean; pin: (open: boolean) => void; trigger: TriggerProps } {
  const [open, setOpen] = useState(false);
  const intent = useMemo(() => hoverIntent(TIMERS, setOpen), []);
  useEffect(() => {
    hearMoves();
    return () => intent.dispose();
  }, [intent]);
  useEffect(() => {
    if (!enabled) intent.close();
  }, [enabled, intent]);
  const pin = (next: boolean) => {
    if (!next) intent.close();
    setOpen(next);
  };
  const trigger: TriggerProps = enabled
    ? {
        onPointerEnter: (e) => intent.enter(e.pointerType),
        onPointerMove: (e) => intent.move(e.pointerType),
        onPointerLeave: (e) => intent.leave(e.pointerType),
        onPointerDown: () => intent.press(),
      }
    : {};
  return { open: open && enabled, pin, trigger };
}

// keepRadixOff stops Radix's own hover timers on the card: the row's handlers decide
const keepRadixOff = (e: PointerEvent) => e.preventDefault();

// TAKE_FOCUS_AFTER lets a menu that opened the card hand the focus back to its button first, whose tooltip would else
// open on that focus and take the Esc meant for the card
const TAKE_FOCUS_AFTER = 60;

// HoverCard sits inside its row (position: relative): an anchor as large as the row, which takes no pointer, and the
// card beside it in the page's portal. Opened by hand (takeFocus), it is a popover and takes the focus.
export default function HoverCard({ open, onClose, takeFocus = false, side = "right", className, children }: { open: boolean; onClose: () => void; takeFocus?: boolean; side?: "right" | "bottom"; className?: string; children: ReactNode }) {
  const host = usePortalHost();
  const content = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open || !takeFocus) return;
    const timer = window.setTimeout(() => content.current?.focus({ preventScroll: true }), TAKE_FOCUS_AFTER);
    return () => window.clearTimeout(timer);
  }, [open, takeFocus]);
  return (
    <Card.Root open={open} onOpenChange={(next) => !next && onClose()}>
      <Card.Trigger asChild>
        <span className="hover-anchor" aria-hidden="true" />
      </Card.Trigger>
      <Card.Portal container={host}>
        <Card.Content
          ref={content}
          tabIndex={-1}
          className={`popover hover-card${className ? ` ${className}` : ""}`}
          side={side}
          align="start"
          sideOffset={8}
          collisionPadding={8}
          hideWhenDetached
          onPointerEnter={keepRadixOff}
          onPointerLeave={keepRadixOff}
          onClick={(e) => e.stopPropagation()}
        >
          {children}
        </Card.Content>
      </Card.Portal>
    </Card.Root>
  );
}
