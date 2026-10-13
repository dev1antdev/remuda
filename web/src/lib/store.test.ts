import { describe, expect, it, vi } from "vitest";
import { createStore } from "./store";

describe("createStore", () => {
  it("holds the state, patches it and tells its listeners", () => {
    const store = createStore({ count: 1, name: "a" });
    const seen = vi.fn();
    const stop = store.subscribe(seen);
    store.set({ count: 2 });
    expect(store.get()).toEqual({ count: 2, name: "a" });
    store.set((s) => ({ count: s.count + 1 }));
    expect(store.get().count).toBe(3);
    expect(seen).toHaveBeenCalledTimes(2);
    stop();
    store.set({ count: 4 });
    expect(seen).toHaveBeenCalledTimes(2);
  });
});
