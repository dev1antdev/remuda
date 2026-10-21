import { useCallback, useRef, useSyncExternalStore } from "react";

// Per-device preferences in localStorage, and the touch test. Every reader of a key sees a write made
// through any other: the terminal's tools in the tab strip and the lens beside them hold the same keys.

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

export const isTouch = typeof matchMedia === "function" && matchMedia("(pointer: coarse)").matches;

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
  values.set(key, value);
  for (const notify of listeners.get(key) ?? []) notify();
}

// the value every hook of a key shares, read from storage once, and who is listening for it
const values = new Map<string, unknown>();
const listeners = new Map<string, Set<() => void>>();

function current<T>(key: string, fallback: T): T {
  if (!values.has(key)) values.set(key, readPref(key, fallback));
  return values.get(key) as T;
}

// usePref is useState that persists and is shared: the setter writes through, and every other usePref of the key follows.
export function usePref<T>(key: string, fallback: T): [T, (next: T | ((prev: T) => T)) => void] {
  const fallbackRef = useRef(fallback);
  fallbackRef.current = fallback;
  const subscribe = useCallback(
    (notify: () => void) => {
      let set = listeners.get(key);
      if (!set) listeners.set(key, (set = new Set()));
      set.add(notify);
      return () => {
        set.delete(notify);
      };
    },
    [key],
  );
  const value = useSyncExternalStore(subscribe, () => current(key, fallbackRef.current));
  const set = useCallback(
    (next: T | ((prev: T) => T)) => {
      const v = typeof next === "function" ? (next as (p: T) => T)(current(key, fallbackRef.current)) : next;
      writePref(key, v);
    },
    [key],
  );
  return [value, set];
}
