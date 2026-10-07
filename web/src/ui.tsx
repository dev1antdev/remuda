// The shared pieces every screen is built from: the dialog (one scrim, one
// header, Escape, focus), the notice (one block, five tones), and the
// small hooks behind them.

import { useCallback, useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import Icon, { type IconName } from "./icons";
import { errMsg } from "./prefs";

// ---- dialog ----

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
  useEffect(() => {
    // focus moves in; the page behind is inert; both undone on close
    const opener = document.activeElement as HTMLElement | null;
    const root = document.getElementById("root");
    root?.setAttribute("inert", "");
    const first = box.current?.querySelector<HTMLElement>("[autofocus], input:not([type=hidden]), textarea, select, button:not(.icon-btn)");
    (first ?? box.current)?.focus();
    return () => {
      root?.removeAttribute("inert");
      opener?.focus?.();
    };
  }, []);
  useEscape(onClose);
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
            {controls}
            <button type="button" className="icon-btn" onClick={onClose} title="Close" aria-label="Close">
              <Icon name="x" size={16} />
            </button>
          </div>
        )}
        {children}
      </Tag>
    </div>
  );
  return createPortal(node, document.body);
}

// Escape closes the topmost thing: a dialog, unless a Select's list is open
// (its own listener runs first and stops the event)
export function useEscape(onClose: () => void) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || document.querySelector(".sel-pop")) return;
      e.stopPropagation();
      onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);
}

// a popover or menu: a pointer outside it, or Escape, closes it
export function useDismiss(open: boolean, close: () => void, ignore: string[] = []) {
  useEffect(() => {
    if (!open) return;
    const outside = (e: Event) => {
      const t = e.target as Element | null;
      if (t && ignore.some((sel) => t.closest(sel))) return;
      close();
    };
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        close();
      }
    };
    document.addEventListener("mousedown", outside, true);
    document.addEventListener("touchstart", outside, { capture: true, passive: true });
    document.addEventListener("keydown", key, true);
    return () => {
      document.removeEventListener("mousedown", outside, true);
      document.removeEventListener("touchstart", outside, true);
      document.removeEventListener("keydown", key, true);
    };
  }, [open, close, ignore.join("|")]); // eslint-disable-line react-hooks/exhaustive-deps
}

// ---- notice ----

export type Tone = "info" | "success" | "warn" | "error" | "hint";

export function Notice({ tone = "info", icon, children, action, onDismiss, className }: {
  tone?: Tone;
  icon?: IconName;
  children: ReactNode;
  action?: ReactNode;
  onDismiss?: () => void;
  className?: string;
}) {
  return (
    <div className={`notice ${tone}${className ? ` ${className}` : ""}`} role={tone === "error" ? "alert" : undefined}>
      {icon && <Icon name={icon} size={14} className="notice-icon" />}
      <span className="notice-text">{children}</span>
      {action}
      {onDismiss && (
        <button type="button" className="icon-btn sm" onClick={onDismiss} title="Dismiss" aria-label="Dismiss">
          <Icon name="x" size={12} />
        </button>
      )}
    </div>
  );
}

// a confirmation that goes away on its own; a later one replaces the timer
export function useTimedNotice(ms = 5000): [string, (text: string) => void, () => void] {
  const [text, setText] = useState("");
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const clear = useCallback(() => {
    clearTimeout(timer.current);
    setText("");
  }, []);
  const show = useCallback(
    (t: string) => {
      clearTimeout(timer.current);
      setText(t);
      timer.current = setTimeout(() => setText(""), ms);
    },
    [ms],
  );
  useEffect(() => () => clearTimeout(timer.current), []);
  return [text, show, clear];
}

// ---- async ----

// a form's submit: busy while it runs, the error as text
export function useSubmit<A extends unknown[]>(fn: (...args: A) => Promise<void>) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const run = useCallback(
    async (...args: A) => {
      setBusy(true);
      setErr("");
      try {
        await fn(...args);
      } catch (e) {
        setErr(errMsg(e));
      } finally {
        setBusy(false);
      }
    },
    [fn],
  );
  return { busy, err, run, setErr };
}

// a fetch that follows its inputs: a result that arrives after they
// changed is dropped
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]): { data: T | null; error: string; loading: boolean; reload: () => void } {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let gone = false;
    setLoading(true);
    fn().then(
      (d) => {
        if (gone) return;
        setData(d);
        setError("");
        setLoading(false);
      },
      (e) => {
        if (gone) return;
        setError(errMsg(e));
        setLoading(false);
      },
    );
    return () => {
      gone = true;
    };
  }, [...deps, tick]); // eslint-disable-line react-hooks/exhaustive-deps
  return { data, error, loading, reload: () => setTick((t) => t + 1) };
}

// ---- small marks ----

export function StatusDot({ status, title }: { status: string; title?: string }) {
  return <span className={`dot dot-${status}`} title={title} />;
}
