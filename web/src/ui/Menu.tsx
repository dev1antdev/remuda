// A menu behind a button: Radix's DropdownMenu in the design system's clothes. The keyboard walks it, typing jumps, Esc
// closes, it opens where there is room. Every row is a word, with an icon, a hint or a shortcut beside it when they help.

import { DropdownMenu } from "radix-ui";
import type { ReactNode } from "react";
import Icon, { type IconName } from "./icons";

export type MenuItem =
  | { label: string; icon?: IconName; hint?: string; shortcut?: string; checked?: boolean; danger?: boolean; disabled?: boolean; onSelect: () => void }
  | { sep: true }
  | { heading: string };

export default function Menu({ trigger, items, align = "end", side = "bottom", label }: { trigger: ReactNode; items: MenuItem[]; align?: "start" | "center" | "end"; side?: "top" | "bottom" | "left" | "right"; label?: string }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="menu" align={align} side={side} sideOffset={6} collisionPadding={8} aria-label={label}>
          {items.map((item, i) =>
            "sep" in item ? (
              <DropdownMenu.Separator key={i} className="menu-sep" />
            ) : "heading" in item ? (
              <DropdownMenu.Label key={i} className="menu-heading">
                {item.heading}
              </DropdownMenu.Label>
            ) : (
              <DropdownMenu.Item key={i} className={`menu-item${item.danger ? " danger" : ""}${item.checked ? " checked" : ""}`} disabled={item.disabled} onSelect={item.onSelect}>
                <span className="menu-icon">{item.checked ? <Icon name="check" size={14} /> : item.icon ? <Icon name={item.icon} size={14} /> : null}</span>
                <span className="menu-text">
                  {item.label}
                  {item.hint && <small>{item.hint}</small>}
                </span>
                {item.shortcut && <kbd className="menu-kbd">{item.shortcut}</kbd>}
              </DropdownMenu.Item>
            ),
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
