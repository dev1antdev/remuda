// A panel behind a button: Radix's Popover in the design system's clothes, for what is more than a list of rows — a
// machine's vitals. It opens where there is room inside the window, scrolls when taller than the room, follows its
// button while the page scrolls — hidden while its button is off the window, as in a drawer slid shut — and closes on
// Esc or a click outside. A click inside stays there, as a menu's does.

import { Popover as Pop } from "radix-ui";
import type { ReactNode } from "react";
import { usePortalHost } from "./portal";
import Tip from "./Tip";

// focusPanel puts the focus on the panel itself, not on its first button, whose tooltip would open with it
function focusPanel(e: Event) {
  e.preventDefault();
  if (e.currentTarget instanceof HTMLElement) e.currentTarget.focus();
}

export default function Popover({ trigger, tip, open, onOpenChange, label, className, children, align = "start", side = "bottom" }: {
  trigger: ReactNode;
  tip?: ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  label: string;
  className?: string;
  children: ReactNode;
  align?: "start" | "center" | "end";
  side?: "top" | "bottom" | "left" | "right";
}) {
  const host = usePortalHost();
  const opener = <Pop.Trigger asChild>{trigger}</Pop.Trigger>;
  return (
    <Pop.Root open={open} onOpenChange={onOpenChange}>
      {tip ? <Tip text={tip}>{opener}</Tip> : opener}
      <Pop.Portal container={host}>
        <Pop.Content className={`popover${className ? ` ${className}` : ""}`} align={align} side={side} sideOffset={6} collisionPadding={8} hideWhenDetached aria-label={label} onClick={(e) => e.stopPropagation()} onOpenAutoFocus={focusPanel}>
          {children}
        </Pop.Content>
      </Pop.Portal>
    </Pop.Root>
  );
}
