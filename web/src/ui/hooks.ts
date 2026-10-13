// The small hooks behind the pieces: a popover's dismissal, a form's submit, a notice that fades, a debounced value.

import { useCallback, useEffect, useRef, useState } from "react";
import { errMsg } from "@/lib/text";

// useDismiss closes a popover or menu on a pointer outside it, or Escape.
export function useDismiss(open: boolean, close: () => void, ignore: string[] = []) {
  useEffect(() => {
    if (!open) return;
    const outside = (e: Event) => {
      const t = e.target as Element | null;
      if (t && ignore.some((sel) => t.closest(sel))) return;
      close();
    };
    const key = (e: KeyboardEvent) => {
      // a Select's open list takes the Escape; its listener is a sibling on the same node, so stopPropagation alone would not keep us out
      if (e.key === "Escape" && !document.querySelector(".sel-pop")) {
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

// useTimedNotice is a confirmation that goes away on its own; a later one replaces the timer.
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

// useSubmit runs a form's submit: busy while it runs, the error as text.
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

// useDebounced is the value as it was ms ago, for a search box.
export function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}
