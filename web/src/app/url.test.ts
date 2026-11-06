import { afterEach, describe, expect, it } from "vitest";
import { addressFor, panelOf, readURL } from "./url";

const g = globalThis as unknown as { location?: unknown };

afterEach(() => {
  delete g.location;
});

describe("addressFor", () => {
  it("names the pane and its lens in Agents, the mode elsewhere, and is bare for the first mode with nothing selected", () => {
    expect(addressFor({ selection: { machineId: "local", paneId: "w1:p1" }, mode: "herdr", lens: "chat" })).toBe("/?machine=local&pane=w1%3Ap1&lens=chat");
    expect(addressFor({ selection: { machineId: "gog", paneId: "w2:p1" }, mode: "wall", lens: "chat" })).toBe("/?machine=gog&pane=w2%3Ap1&mode=wall");
    expect(addressFor({ selection: null, mode: "tasks", lens: "terminal" })).toBe("/?mode=tasks");
    expect(addressFor({ selection: null, mode: "herdr", lens: "terminal" })).toBe("/");
  });
  it("builds the same address on a browser whose URLSearchParams has no size (Safari 16)", () => {
    const size = Object.getOwnPropertyDescriptor(URLSearchParams.prototype, "size");
    if (size) delete (URLSearchParams.prototype as { size?: number }).size;
    try {
      expect(new URLSearchParams("a=1").size).toBeUndefined();
      expect(addressFor({ selection: { machineId: "local", paneId: "w1:p1" }, mode: "herdr", lens: "terminal" })).toBe("/?machine=local&pane=w1%3Ap1&lens=terminal");
      expect(addressFor({ selection: null, mode: "herdr", lens: "terminal" })).toBe("/");
    } finally {
      if (size) Object.defineProperty(URLSearchParams.prototype, "size", size);
    }
  });
});

describe("readURL", () => {
  it("reads the pane, the mode, the lens, a panel and a turn's link off the address", () => {
    g.location = { search: "?machine=gog&pane=w1%3Ap1&mode=wall&lens=chat&panel=standup&session=abc&turn=u1" };
    expect(readURL()).toEqual({
      requested: { machineId: "gog", paneId: "w1:p1" },
      mode: "wall",
      lens: "chat",
      panel: "standup",
      turn: { machineId: "gog", paneId: "w1:p1", session: "abc", turn: "u1" },
    });
  });
  it("takes the local machine for a pane without one and drops what it does not know", () => {
    g.location = { search: "?pane=w1%3Ap1&mode=sideways&lens=x&panel=palette" };
    expect(readURL()).toEqual({ requested: { machineId: "local", paneId: "w1:p1" }, mode: undefined, lens: undefined, panel: null, turn: null });
    expect(panelOf("palette")).toBeNull();
    expect(panelOf("alerts")).toBe("alerts");
  });
});
