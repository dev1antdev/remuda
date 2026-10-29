import { describe, expect, it } from "vitest";
import { canSplit, shownLenses, splitOn } from "./split";

describe("split view", () => {
  it("needs a wide screen and a mouse", () => {
    expect(canSplit(true, false)).toBe(true);
    expect(canSplit(false, false)).toBe(false);
    expect(canSplit(true, true)).toBe(false);
  });
  it("shows both sides only when asked, possible and not on the Changes lens", () => {
    expect(splitOn(true, true, false, "chat")).toBe(true);
    expect(splitOn(true, true, false, "terminal")).toBe(true);
    expect(splitOn(true, true, false, "changes")).toBe(false);
    expect(splitOn(false, true, false, "chat")).toBe(false);
    expect(splitOn(true, false, false, "chat")).toBe(false);
  });
  it("lists what is on screen", () => {
    expect(shownLenses("chat", true)).toEqual(["chat", "terminal"]);
    expect(shownLenses("terminal", false)).toEqual(["terminal"]);
    expect(shownLenses("changes", false)).toEqual(["changes"]);
  });
});
