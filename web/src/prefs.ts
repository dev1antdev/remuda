import { useCallback, useState } from "react";
import type { Prefs } from "./push";

// Small shared things: per-device preferences in localStorage, the touch
// test, and a couple of one-liners every component was re-typing.

export const isTouch = matchMedia("(pointer: coarse)").matches;

export function readPref<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(`herdr-ui.${key}`);
    return v === null ? fallback : (JSON.parse(v) as T);
  } catch {
    return fallback;
  }
}

export function writePref(key: string, value: unknown): void {
  try {
    localStorage.setItem(`herdr-ui.${key}`, JSON.stringify(value));
  } catch {
    /* private mode etc. */
  }
}

// usePref is useState that persists: the setter writes through.
export function usePref<T>(key: string, fallback: T): [T, (next: T | ((prev: T) => T)) => void] {
  const [value, setValue] = useState<T>(() => readPref(key, fallback));
  const set = useCallback(
    (next: T | ((prev: T) => T)) => {
      setValue((prev) => {
        const v = typeof next === "function" ? (next as (p: T) => T)(prev) : next;
        writePref(key, v);
        return v;
      });
    },
    [key],
  );
  return [value, set];
}

export const DEFAULT_ALERT_PREFS: Prefs = { input: true, done: "long", watchdog: true };

export function readAlertPrefs(): Prefs {
  return { ...DEFAULT_ALERT_PREFS, ...readPref<Partial<Prefs>>("alerts", {}) };
}

export function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function basename(p: string): string {
  const parts = p.split("/");
  return parts[parts.length - 1] || p;
}
