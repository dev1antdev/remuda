// A select that can show an icon beside each option — a native <select>
// cannot. A button shows the chosen option; the list opens in a portal at
// the button (fixed, flipping upward when the screen runs out), so no
// scrolling parent clips it. Arrow keys move, Enter picks, Escape closes.

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

export default function Select<T extends string | number>({ value, options, onChange, className, placeholder, title }: {
  value: T;
  options: Option<T>[];
  onChange: (v: T) => void;
  className?: string;
  placeholder?: string;
  title?: string;
}) {
  const [open, setOpen] = useState(false);
  const [at, setAt] = useState<{ x: number; y: number; w: number; up: boolean } | null>(null);
  const [focus, setFocus] = useState(-1);
  const btn = useRef<HTMLButtonElement>(null);
  const pop = useRef<HTMLDivElement>(null);
  const chosen = options.find((o) => o.value === value);

  const place = () => {
    const r = btn.current?.getBoundingClientRect();
    if (!r) return;
    const want = Math.min(320, 8 + options.length * 30 + 8);
    const up = r.bottom + want > window.innerHeight - 8 && r.top > want;
    setAt({ x: r.left, y: up ? r.top : r.bottom, w: r.width, up });
  };
  const show = () => {
    place();
    setFocus(Math.max(0, options.findIndex((o) => o.value === value)));
    setOpen(true);
  };
  const hide = () => {
    setOpen(false);
    btn.current?.focus();
  };

  useLayoutEffect(() => {
    if (!open) return;
    const onAway = (e: MouseEvent) => {
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
    document.addEventListener("keydown", onKey, true);
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      document.removeEventListener("mousedown", onAway, true);
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
    setOpen(false);
    btn.current?.focus();
    if (o.value !== value) onChange(o.value);
  };
  const enabled = options.filter((o) => !o.disabled);
  const move = (d: number) => setFocus((f) => (enabled.length ? (f + d + enabled.length) % enabled.length : -1));

  // group headers in first-seen order
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
        title={title}
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
            style={{ left: at.x, top: at.up ? undefined : at.y, bottom: at.up ? window.innerHeight - at.y : undefined, minWidth: Math.max(at.w, 180) }}
            role="listbox"
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault();
                move(e.key === "ArrowDown" ? 1 : -1);
              } else if (e.key === "Tab") {
                setOpen(false);
              }
            }}
          >
            {groups.map((g, gi) => (
              <div key={g.name ?? gi} className="sel-group">
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
                    {o.value === value && <span className="sel-check">✓</span>}
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
