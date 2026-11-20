import { beforeAll, describe, expect, it, vi } from "vitest";

// a storage of its own for the module: the test never reaches a real one
const kept = new Map<string, string>();
const storage = {
  getItem: (key: string) => kept.get(key) ?? null,
  setItem: (key: string, value: string) => void kept.set(key, String(value)),
  removeItem: (key: string) => void kept.delete(key),
  clear: () => kept.clear(),
  key: (i: number) => [...kept.keys()][i] ?? null,
  get length() {
    return kept.size;
  },
};

let prefs: typeof import("./prefs");

beforeAll(async () => {
  vi.stubGlobal("localStorage", new Proxy(storage, { ownKeys: () => [...kept.keys()], getOwnPropertyDescriptor: (_, key) => (kept.has(String(key)) ? { enumerable: true, configurable: true, value: kept.get(String(key)) } : undefined) }));
  prefs = await import("./prefs");
});

describe("a pref another device set, applied here", () => {
  const BAR = { on: true, remaining: false };
  it("reaches every hook of the key in the hook's own shape: fields an older build lacked come from the fallback", () => {
    expect(prefs.prefValue("statusbar", BAR)).toEqual(BAR);
    prefs.applyPref("statusbar", { on: false }, "2026-11-01T10:00:00.000Z");
    expect(prefs.prefValue("statusbar", BAR)).toEqual({ on: false, remaining: false });
    expect(prefs.sharedPrefs().statusbar).toEqual({ value: { on: false }, at: "2026-11-01T10:00:00.000Z" });
  });
  it("of another type altogether is dropped for the fallback, as a stored one is", () => {
    expect(prefs.prefValue("theme.dark", "frappe")).toBe("frappe");
    prefs.applyPref("theme.dark", 42, "2026-11-01T10:00:00.000Z");
    expect(prefs.prefValue("theme.dark", "frappe")).toBe("frappe");
    prefs.applyPref("theme.dark", "mocha", "2026-11-01T11:00:00.000Z");
    expect(prefs.prefValue("theme.dark", "frappe")).toBe("mocha");
  });
  it("leaves a write made here as it was written", () => {
    prefs.writePref("density", "compact");
    expect(prefs.prefValue("density", "comfortable")).toBe("compact");
  });
});
