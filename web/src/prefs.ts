import { useCallback, useEffect, useState } from "react";
import type { Prefs } from "./push";

// Small shared things: per-device preferences in localStorage, the touch
// test, and a couple of one-liners every component was re-typing.

export const isTouch = matchMedia("(pointer: coarse)").matches;

// readPref keeps a stored value only when it has the fallback's shape: a
// pref saved by an older build may lack fields (merged in from the
// fallback) or be a different type altogether (dropped).
export function readPref<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(`herdr-ui.${key}`);
    if (v === null) return fallback;
    const parsed: unknown = JSON.parse(v);
    if (parsed === null || typeof parsed !== typeof fallback || Array.isArray(parsed) !== Array.isArray(fallback)) return fallback;
    if (typeof fallback === "object" && fallback !== null && !Array.isArray(fallback)) return { ...fallback, ...(parsed as object) } as T;
    return parsed as T;
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

// the last path segment ("" for a bare "/"), a trailing slash ignored
export const basename = (p: string) => p.split("/").filter(Boolean).pop() ?? p;
export const dirname = (p: string) => {
  const i = p.lastIndexOf("/");
  return i < 0 ? "" : p.slice(0, i + 1);
};

// a value that settles: the input as it was ms ago, for a search box
export function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}
