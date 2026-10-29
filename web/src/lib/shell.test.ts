import { afterEach, describe, expect, it } from "vitest";
import { handShell, inShell, tellShell } from "./shell";

type FakeWindow = { remudaShell?: Record<string, unknown>; webkit?: { messageHandlers?: { remuda?: { postMessage: (m: unknown) => void } } } };
const g = globalThis as unknown as { window?: FakeWindow };

afterEach(() => {
  delete g.window;
});

describe("inShell", () => {
  it("is false without a window, without the app, and with an app that is not native", () => {
    expect(inShell()).toBe(false);
    g.window = {};
    expect(inShell()).toBe(false);
    g.window = { remudaShell: { native: false } };
    expect(inShell()).toBe(false);
  });
  it("is true once the app announced itself", () => {
    g.window = { remudaShell: { native: true, version: "0.1" } };
    expect(inShell()).toBe(true);
  });
});

describe("tellShell", () => {
  it("posts to the app's handler and says so", () => {
    const got: unknown[] = [];
    g.window = { remudaShell: { native: true }, webkit: { messageHandlers: { remuda: { postMessage: (m) => got.push(m) } } } };
    expect(tellShell({ type: "badge", count: 2, tone: "red" })).toBe(true);
    expect(got).toEqual([{ type: "badge", count: 2, tone: "red" }]);
  });
  it("is a quiet no-op without a handler, and never throws", () => {
    expect(tellShell({ type: "ready", version: "v", machine: "m" })).toBe(false);
    g.window = { webkit: {} };
    expect(tellShell({ type: "ready", version: "v", machine: "m" })).toBe(false);
    g.window = {
      webkit: {
        messageHandlers: {
          remuda: {
            postMessage: () => {
              throw new Error("gone");
            },
          },
        },
      },
    };
    expect(tellShell({ type: "menu", rows: [], working: 0, idle: 0 })).toBe(false);
  });
});

describe("handShell", () => {
  it("adds the page's functions beside the app's flag and does nothing without the app", () => {
    const select = () => {};
    handShell({ select });
    g.window = {};
    handShell({ select });
    expect(g.window.remudaShell).toBeUndefined();
    g.window = { remudaShell: { native: true, version: "0.1" } };
    handShell({ select });
    expect(g.window.remudaShell).toEqual({ native: true, version: "0.1", select });
  });
});
