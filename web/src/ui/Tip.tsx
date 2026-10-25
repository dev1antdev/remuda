// A tooltip that says what a control does: Radix's, so it waits a beat, follows the keyboard focus, never opens on a touch,
// and sits where there is room. One provider over the page shares the delay, so moving along a bar opens them at once.

import { Tooltip } from "radix-ui";
import type { ReactElement, ReactNode } from "react";

export function TipProvider({ children }: { children: ReactNode }) {
  return (
    <Tooltip.Provider delayDuration={450} skipDelayDuration={400}>
      {children}
    </Tooltip.Provider>
  );
}

export default function Tip({ text, side = "bottom", children }: { text: ReactNode; side?: "top" | "bottom" | "left" | "right"; children: ReactElement }) {
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{children}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content className="tip" side={side} sideOffset={6} collisionPadding={8}>
          {text}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}
