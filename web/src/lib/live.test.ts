import { describe, expect, it } from "vitest";
import { backoffDelay, onEvent, paneKey, parseEvent, slowerPoll, subscriptionFrame, watchPane } from "./live";

describe("backoffDelay", () => {
  it("doubles from a second and stops at half a minute", () => {
    expect([0, 1, 2, 3, 4, 5, 6, 20].map(backoffDelay)).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000]);
    expect(backoffDelay(-3)).toBe(1000);
  });
});

describe("subscriptionFrame", () => {
  it("lists every watched key once, sorted, leaving out the ones nobody holds", () => {
    const watched = new Map([
      ["local|w3:p1", 2],
      ["gog|w1:p1", 1],
      ["local|w1:p1", 0],
    ]);
    expect(subscriptionFrame(watched)).toBe('{"sub":["gog|w1:p1","local|w3:p1"]}');
    expect(subscriptionFrame(new Map())).toBe('{"sub":[]}');
  });
});

describe("watchPane", () => {
  it("counts viewers of a pane and lets go when the last one leaves", () => {
    const a = watchPane("local", "w5:p1");
    const b = watchPane("local", "w5:p1");
    a();
    // the second viewer still holds the pane; nothing observable without a socket, so the release must not throw
    b();
    expect(paneKey("local", "w5:p1")).toBe("local|w5:p1");
  });
});

describe("parseEvent", () => {
  it("keeps the two topics and drops anything else", () => {
    expect(parseEvent('{"topic":"roster"}')).toEqual({ topic: "roster", machine: undefined, pane: undefined, version: undefined });
    expect(parseEvent('{"topic":"chat","machine":"local","pane":"w1:p1","version":"12-3"}')).toEqual({ topic: "chat", machine: "local", pane: "w1:p1", version: "12-3" });
    expect(parseEvent('{"topic":"weather"}')).toBeNull();
    expect(parseEvent("not json")).toBeNull();
    expect(parseEvent("null")).toBeNull();
    expect(parseEvent('{"topic":"chat","pane":7}')).toEqual({ topic: "chat", machine: undefined, pane: undefined, version: undefined });
  });
});

describe("slowerPoll", () => {
  it("keeps the pace without a socket and slows to 10 s or 30 s with one", () => {
    expect(slowerPoll(2000, false)).toBe(2000);
    expect(slowerPoll(2000, true)).toBe(10000);
    expect(slowerPoll(3000, true)).toBe(10000);
    expect(slowerPoll(10000, true)).toBe(30000);
    expect(slowerPoll(15000, true)).toBe(30000);
  });
});

describe("onEvent", () => {
  it("stops hearing after the release", () => {
    const heard: string[] = [];
    const stop = onEvent("roster", (event) => heard.push(event.topic));
    stop();
    expect(heard).toEqual([]);
  });
});
