// A select that can show an icon beside each option, which a native <select> cannot: a button shows
// the chosen option and the list opens in a portal at the button, fixed and kept inside the viewport
// (flipping upward when the screen runs out), so no scrolling parent clips it.

import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import Icon from "./icons";

export type Option<T extends string | number> = {
  value: T;
  label: string;
  icon?: ReactNode;
  hint?: string; // second-rank text after the label
  group?: string; // options with the same group sit under one header
  disabled?: boolean;
};

const MIN_WIDTH = 180;
const MAX_HEIGHT = 320;
const EDGE = 8; // the list stays this far from the viewport's edge

export default function Select<T extends string | number>({ value, options, onChange, className, placeholder }: {
  value: T;
  options: Option<T>[];
  onChange: (v: T) => void;
  className?: string;
  placeholder?: string;
}) {
  const [open, setOpen] = useState(false);
  const [at, setAt] = useState<{ x: number; y: number; w: number; up: boolean; maxH: number } | null>(null);
  const [focus, setFocus] = useState(-1); // an index into the enabled options
  const btn = useRef<HTMLButtonElement>(null);
  const pop = useRef<HTMLDivElement>(null);
  const chosen = options.find((o) => o.value === value);
  const enabled = options.filter((o) => !o.disabled);

  const place = () => {
    const r = btn.current?.getBoundingClientRect();
    if (!r) return;
    const want = Math.min(MAX_HEIGHT, 8 + options.length * 30 + 8);
    const below = window.innerHeight - r.bottom - EDGE;
    const above = r.top - EDGE;
    const up = want > below && above > below;
    const w = Math.max(r.width, MIN_WIDTH);
    setAt({
      x: Math.max(EDGE, Math.min(r.left, window.innerWidth - EDGE - w)),
      y: up ? r.top : r.bottom,
      w,
      up,
      maxH: Math.max(120, Math.min(MAX_HEIGHT, up ? above : below)),
    });
  };
  const show = () => {
    place();
    setFocus(Math.max(0, enabled.findIndex((o) => o.value === value)));
    setOpen(true);
  };
  const hide = () => {
    setOpen(false);
    btn.current?.focus();
  };

  useLayoutEffect(() => {
    if (!open) return;
    const onAway = (e: Event) => {
      if (pop.current?.contains(e.target as Node) || btn.current?.contains(e.target as Node)) return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        hide();
      }
    };
    document.addEventListener("mousedown", onAway, true);
    document.addEventListener("touchstart", onAway, { capture: true, passive: true });
    document.addEventListener("keydown", onKey, true);
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      document.removeEventListener("mousedown", onAway, true);
      document.removeEventListener("touchstart", onAway, true);
      document.removeEventListener("keydown", onKey, true);
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!open) return;
    const el = pop.current?.querySelectorAll<HTMLButtonElement>("button:not(:disabled)")[focus];
    el?.focus();
    el?.scrollIntoView({ block: "nearest" });
  }, [open, focus]);

  const pick = (o: Option<T>) => {
    if (o.disabled) return;
    hide();
    if (o.value !== value) onChange(o.value);
  };
  const move = (d: number) => setFocus((f) => (enabled.length ? (f + d + enabled.length) % enabled.length : -1));

  const groups: { name: string | undefined; items: Option<T>[] }[] = [];
  for (const o of options) {
    const g = groups.find((x) => x.name === o.group);
    if (g) g.items.push(o);
    else groups.push({ name: o.group, items: [o] });
  }

  return (
    <>
      <button
        ref={btn}
        type="button"
        className={`sel${open ? " open" : ""}${className ? ` ${className}` : ""}`}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => (open ? setOpen(false) : show())}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            if (!open) show();
            else move(e.key === "ArrowDown" ? 1 : -1);
          }
        }}
      >
        {chosen?.icon && <span className="sel-icon">{chosen.icon}</span>}
        <span className="sel-label">{chosen ? chosen.label : (placeholder ?? "—")}</span>
        <span className="sel-chev" aria-hidden="true">
          <Icon name="chevron-down" size={12} />
        </span>
      </button>
      {open &&
        at &&
        createPortal(
          <div
            ref={pop}
            className={`sel-pop${at.up ? " up" : ""}`}
            style={{
              left: at.x,
              top: at.up ? undefined : at.y,
              bottom: at.up ? window.innerHeight - at.y : undefined,
              minWidth: at.w,
              maxWidth: window.innerWidth - 2 * EDGE,
              maxHeight: at.maxH,
            }}
            role="listbox"
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault();
                move(e.key === "ArrowDown" ? 1 : -1);
              } else if (e.key === "Tab") {
                hide(); // focus returns to the button, then Tab moves on from there
              }
            }}
          >
            {groups.map((g, gi) => (
              <div key={g.name ?? gi} role={g.name ? "group" : undefined} aria-label={g.name}>
                {g.name && <div className="sel-group-name">{g.name}</div>}
                {g.items.map((o) => (
                  <button
                    key={String(o.value)}
                    type="button"
                    role="option"
                    aria-selected={o.value === value}
                    className={`sel-opt${o.value === value ? " on" : ""}`}
                    disabled={o.disabled}
                    onMouseEnter={() => setFocus(enabled.indexOf(o))}
                    onClick={() => pick(o)}
                  >
                    {o.icon !== undefined && <span className="sel-icon">{o.icon}</span>}
                    <span className="sel-label">{o.label}</span>
                    {o.hint && <span className="sel-hint">{o.hint}</span>}
                    {o.value === value && (
                      <span className="sel-check">
                        <Icon name="check" size={12} />
                      </span>
                    )}
                  </button>
                ))}
              </div>
            ))}
          </div>,
          document.body,
        )}
    </>
  );
}
