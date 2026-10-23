import { describe, expect, it } from "vitest";
import { recall, remember } from "./history";

describe("remember", () => {
  it("puts the newest first, moves a repeat up and keeps the list short", () => {
    expect(remember([], "go")).toEqual(["go"]);
    expect(remember(["a", "b"], "c")).toEqual(["c", "a", "b"]);
    expect(remember(["a", "b", "c"], "b")).toEqual(["b", "a", "c"]);
    expect(remember(["a"], "  a  ")).toEqual(["a"]);
    expect(remember(["a"], "   ")).toEqual(["a"]);
    expect(remember(Array.from({ length: 50 }, (_, i) => `p${i}`), "new")).toHaveLength(50);
  });
});

describe("recall", () => {
  const list = ["newest", "older", "oldest"];
  it("walks up to the oldest and stops there", () => {
    expect(recall(list, -1, "up", "")).toEqual({ at: 0, text: "newest" });
    expect(recall(list, 0, "up", "")).toEqual({ at: 1, text: "older" });
    expect(recall(list, 2, "up", "")).toBeNull();
    expect(recall([], -1, "up", "")).toBeNull();
  });
  it("walks down back to the draft that was being written", () => {
    expect(recall(list, 2, "down", "half a line")).toEqual({ at: 1, text: "older" });
    expect(recall(list, 0, "down", "half a line")).toEqual({ at: -1, text: "half a line" });
    expect(recall(list, -1, "down", "half a line")).toBeNull();
  });
});
