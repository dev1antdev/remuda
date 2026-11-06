// The dialog: one scrim, one header, Escape, focus in and back out, the page behind it inert.

import { useCallback, useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import Icon, { type IconName } from "@/ui/icons";
import Tip from "@/ui/Tip";
import { dialogTakesEscape } from "./dialogEscape";
import { backTo, focusReturn } from "./dialogFocus";
import { t } from "@/lib/i18n";

// openDialogs counts the open dialogs: the page behind them is inert until the last one closes.
let openDialogs = 0;
// returnsTo is where each open dialog, by its box, gives the focus back: a dialog opened from inside it takes that place
// once its own opener has left the page with it (the palette's field, for a dialog the palette opened)
const returnsTo = new WeakMap<Element, HTMLElement | null>();

export function Dialog({ title, icon, size = "sm", as = "div", onClose, onSubmit, controls, children, className, labelledBy }: {
  title?: ReactNode;
  icon?: IconName;
  size?: "sm" | "md" | "lg";
  as?: "div" | "form";
  onClose: () => void;
  onSubmit?: (e: FormEvent) => void;
  controls?: ReactNode; // header controls, left of the X
  children: ReactNode;
  className?: string;
  labelledBy?: string; // when the title is rendered by the caller
}) {
  const id = useId();
  const box = useRef<HTMLDivElement & HTMLFormElement>(null);
  // the element to give the focus back to, read before the first render: an autoFocus field has it by the time an effect
  // runs; for a menu's row, the menu's button; and where the dialog it was opened from would send it (dialogFocus.ts)
  const [back] = useState(() => {
    const active = document.activeElement as HTMLElement | null;
    const outer = active?.closest('[role="dialog"]');
    return { opener: focusReturn(active, (id) => document.getElementById(id)), outer: outer ? (returnsTo.get(outer) ?? null) : null };
  });
  useEffect(() => {
    const root = document.getElementById("root");
    openDialogs++;
    root?.setAttribute("inert", "");
    if (box.current) returnsTo.set(box.current, back.opener);
    // React's autoFocus focuses without writing the attribute: a field it already focused keeps the focus
    if (!box.current?.contains(document.activeElement)) {
      const first = box.current?.querySelector<HTMLElement>("input:not([type=hidden]), textarea, select, button:not(.icon-btn)");
      (first ?? box.current)?.focus();
    }
    return () => {
      if (--openDialogs === 0) root?.removeAttribute("inert");
      backTo(back.opener, back.outer)?.focus?.();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  // Escape closes this dialog only when it is the topmost one
  const topmost = useCallback(() => {
    const scrims = document.querySelectorAll(".modal-scrim");
    return scrims[scrims.length - 1] === box.current?.parentElement;
  }, []);
  useEscape(onClose, topmost);
  const Tag = as;
  const node = (
    <div className="modal-scrim" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <Tag
        ref={box}
        className={`modal ${size}${className ? ` ${className}` : ""}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? id : labelledBy}
        tabIndex={-1}
        onSubmit={onSubmit}
      >
        {(title || controls) && (
          <div className="modal-head">
            {title && (
              <h2 id={id}>
                {icon && <Icon name={icon} size={16} />} {title}
              </h2>
            )}
            {controls && <div className="modal-head-controls">{controls}</div>}
            <Tip text={t("Close")}>
              <button type="button" className="icon-btn" onClick={onClose} aria-label={t("Close")}>
                <Icon name="x" size={16} />
              </button>
            </Tip>
          </div>
        )}
        {children}
      </Tag>
    </div>
  );
  return createPortal(node, document.body);
}

// useEscape closes the topmost thing on Escape: not while a Select's list is open (its own listener
// runs first and stops the event), not when `when` says another layer is above this one, and not for the
// Esc a tooltip or a menu inside it already used to close itself (dialogEscape.ts).
export function useEscape(onClose: () => void, when?: () => boolean) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!dialogTakesEscape({ key: e.key, defaultPrevented: e.defaultPrevented }, !!document.querySelector(".sel-pop"), !when || when())) return;
      e.stopPropagation();
      onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose, when]);
}
