import { afterEach, describe, expect, it } from "vitest";
import { tellAppPrefs } from "./prefs";

type FakeWindow = { remudaShell?: Record<string, unknown>; webkit?: { messageHandlers?: { remuda?: { postMessage: (m: unknown) => void } } } };
const g = globalThis as unknown as { window?: FakeWindow };

afterEach(() => {
  delete g.window;
});

describe("tellAppPrefs", () => {
  const prefs = { input: false, done: "always" as const, watchdog: true, quiet: { from: "22:00", to: "07:00", tz: "Europe/Kyiv" } };

  it("hands this device's switches and quiet hours to the Mac app, whose alerts the server sends by them", () => {
    const got: unknown[] = [];
    g.window = { remudaShell: { native: true, version: "0.12" }, webkit: { messageHandlers: { remuda: { postMessage: (m) => got.push(m) } } } };
    expect(tellAppPrefs(prefs)).toBe(true);
    expect(got).toEqual([{ type: "alerts", prefs }]);
  });

  it("does nothing in a browser, whose own push subscription carries them", () => {
    expect(tellAppPrefs(prefs)).toBe(false);
    const got: unknown[] = [];
    g.window = { webkit: { messageHandlers: { remuda: { postMessage: (m) => got.push(m) } } } };
    expect(tellAppPrefs(prefs)).toBe(false);
    expect(got).toEqual([]);
  });
});
