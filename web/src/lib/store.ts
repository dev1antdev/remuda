// A store is one object any code may set and any component may read a field of; a change wakes only the
// readers whose field changed. No reducer, no actions: a set is a patch.

import { useSyncExternalStore } from "react";

export type Store<S extends object> = {
  get: () => S;
  set: (patch: Partial<S> | ((prev: S) => Partial<S>)) => void;
  subscribe: (listener: () => void) => () => void;
  // use reads one field (or something stable derived from it) and re-renders when it changes
  use: <T>(select: (s: S) => T) => T;
};

export function createStore<S extends object>(initial: S): Store<S> {
  let state = initial;
  const listeners = new Set<() => void>();
  const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  };
  const set: Store<S>["set"] = (patch) => {
    const next = typeof patch === "function" ? patch(state) : patch;
    let changed = false;
    for (const key in next) {
      if (!Object.is(state[key], next[key])) {
        changed = true;
        break;
      }
    }
    if (!changed) return;
    state = { ...state, ...next };
    for (const listener of listeners) listener();
  };
  return {
    get: () => state,
    set,
    subscribe,
    use: (select) => useSyncExternalStore(subscribe, () => select(state), () => select(state)),
  };
}
