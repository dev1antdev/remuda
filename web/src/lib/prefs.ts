import { useCallback, useState } from "react";

// Per-device preferences in localStorage, and the touch test.

// The keys of the previous name (herdr-ui.*) carry over once.
try {
  for (const k of Object.keys(localStorage)) {
    if (!k.startsWith("herdr-ui.")) continue;
    const nk = "remuda." + k.slice("herdr-ui.".length);
    const v = localStorage.getItem(k);
    if (v !== null && localStorage.getItem(nk) === null) localStorage.setItem(nk, v);
    localStorage.removeItem(k);
  }
} catch {
  // no storage here
}

export const isTouch = matchMedia("(pointer: coarse)").matches;

// readPref keeps a stored value only in the fallback's shape: a pref saved by an older build may lack
// fields (merged in from the fallback) or be a different type altogether (dropped).
export function readPref<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(`remuda.${key}`);
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
    localStorage.setItem(`remuda.${key}`, JSON.stringify(value));
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

