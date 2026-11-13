// A menu behind a button: Radix's DropdownMenu in the design system's clothes. The keyboard walks it, typing jumps, Esc
// closes, it opens where there is room — inside the window, scrolling when it is taller than the room — and follows its
// button. Every row is a word, with an icon, a hint or a shortcut beside it when they help; a row that steps a value
// (keepOpen) leaves the menu open for the next step.

import { DropdownMenu } from "radix-ui";
import type { ReactNode } from "react";
import Icon, { type IconName } from "./icons";
import { usePortalHost } from "./portal";
import Tip from "./Tip";

export type MenuItem =
  | { label: string; icon?: IconName; hint?: string; shortcut?: string; checked?: boolean; danger?: boolean; disabled?: boolean; keepOpen?: boolean; onSelect: () => void }
  | { sep: true }
  | { heading: string };

// items may be a function: a row's menu then works its rows out only when it opens, not at every redraw of the row. A
// click in the menu stays there: React carries a portal's events up its own tree, into the row the button sits in.
// open and onOpenChange hold it from outside, for a row that opens its menu on a right-click too.
export default function Menu({ trigger, tip, items, align = "end", side = "bottom", label, open, onOpenChange }: { trigger: ReactNode; tip?: ReactNode; items: MenuItem[] | (() => MenuItem[]); align?: "start" | "center" | "end"; side?: "top" | "bottom" | "left" | "right"; label?: string; open?: boolean; onOpenChange?: (open: boolean) => void }) {
  const opener = <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>;
  const host = usePortalHost();
  return (
    <DropdownMenu.Root modal={false} open={open} onOpenChange={onOpenChange}>
      {tip ? <Tip text={tip}>{opener}</Tip> : opener}
      <DropdownMenu.Portal container={host}>
        <DropdownMenu.Content className="menu" align={align} side={side} sideOffset={6} collisionPadding={8} hideWhenDetached aria-label={label} onClick={(e) => e.stopPropagation()}>
          <MenuRows items={typeof items === "function" ? items() : items} />
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

function MenuRows({ items }: { items: MenuItem[] }) {
  return (
    <>
      {items.map((item, i) =>
        "sep" in item ? (
          <DropdownMenu.Separator key={i} className="menu-sep" />
        ) : "heading" in item ? (
          <DropdownMenu.Label key={i} className="menu-heading">
            {item.heading}
          </DropdownMenu.Label>
        ) : (
          <DropdownMenu.Item
            key={i}
            className={`menu-item${item.danger ? " danger" : ""}${item.checked ? " checked" : ""}`}
            disabled={item.disabled}
            onSelect={(e) => {
              if (item.keepOpen) e.preventDefault();
              item.onSelect();
            }}
          >
            <span className="menu-icon">{item.checked ? <Icon name="check" size={14} /> : item.icon ? <Icon name={item.icon} size={14} /> : null}</span>
            <span className="menu-text">
              {item.label}
              {item.hint && <small>{item.hint}</small>}
            </span>
            {item.shortcut && <kbd className="menu-kbd">{item.shortcut}</kbd>}
          </DropdownMenu.Item>
        ),
      )}
    </>
  );
}
