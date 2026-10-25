import { useCallback, useRef, useSyncExternalStore } from "react";
import { isShared, type Stamped } from "./syncKeys";

// Per-device preferences in localStorage, and the touch test. Every reader of a key sees a write made
// through any other: the terminal's tools in the tab strip and the lens beside them hold the same keys.
// A shared key (syncKeys.ts) also keeps the moment it was set, so the sync (features/settings/useSyncPrefs.ts)
// can tell which device's copy is newer.

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
// isMac says which modifier a shortcut shows and listens for: ⌘ there, Ctrl everywhere else
export const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

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
  store(key, value);
  if (isShared(key)) {
    setStamp(key, new Date().toISOString());
    for (const notify of writeListeners) notify(key);
  }
}

// applyPref is a write that came from another device through the server: stored with the stamp it was set at, every hook of
// the key woken, the sync layer not told (it is the one calling)
export function applyPref(key: string, value: unknown, at: string): void {
  store(key, value);
  setStamp(key, at);
}

function store(key: string, value: unknown): void {
  try {
    localStorage.setItem(`remuda.${key}`, JSON.stringify(value));
  } catch {
    /* private mode etc. */
  }
  values.set(key, value);
  for (const notify of listeners.get(key) ?? []) notify();
}

// the moment each shared key was last set, kept beside the prefs so a sync can tell which copy is newer
const STAMPS = "remuda.__stamps";

function stamps(): Record<string, string> {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(STAMPS) ?? "{}");
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? (parsed as Record<string, string>) : {};
  } catch {
    return {};
  }
}

function setStamp(key: string, at: string): void {
  try {
    localStorage.setItem(STAMPS, JSON.stringify({ ...stamps(), [key]: at }));
  } catch {
    /* no storage here */
  }
}

// sharedPrefs is every shared key this device holds, with its value and stamp ("" for one set before stamps existed)
export function sharedPrefs(): Record<string, Stamped> {
  const out: Record<string, Stamped> = {};
  const at = stamps();
  try {
    for (const stored of Object.keys(localStorage)) {
      if (!stored.startsWith("remuda.")) continue;
      const key = stored.slice("remuda.".length);
      if (!isShared(key)) continue;
      const raw = localStorage.getItem(stored);
      if (raw === null) continue;
      try {
        out[key] = { value: JSON.parse(raw), at: at[key] ?? "" };
      } catch {
        /* not JSON: not a pref of ours */
      }
    }
  } catch {
    /* no storage here */
  }
  return out;
}

const writeListeners = new Set<(key: string) => void>();

// watchWrites calls back with the key on every write of a shared pref made on this device (not on one applied from the server)
export function watchWrites(notify: (key: string) => void): () => void {
  writeListeners.add(notify);
  return () => {
    writeListeners.delete(notify);
  };
}

// the value every hook of a key shares, read from storage once, and who is listening for it
const values = new Map<string, unknown>();
const listeners = new Map<string, Set<() => void>>();

// another tab's write: the cached value goes and every hook of the key reads storage again
if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (!event.key?.startsWith("remuda.")) return;
    const key = event.key.slice("remuda.".length);
    values.delete(key);
    for (const notify of listeners.get(key) ?? []) notify();
  });
}

// watchPref calls back on every write of the key, through any hook or writePref, and on another tab's write
export function watchPref(key: string, notify: () => void): () => void {
  let set = listeners.get(key);
  if (!set) listeners.set(key, (set = new Set()));
  set.add(notify);
  return () => {
    set.delete(notify);
  };
}

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
