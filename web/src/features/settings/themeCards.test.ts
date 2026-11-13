import { describe, expect, it } from "vitest";
import { previewHues, stepChoice } from "./themeCards";

describe("a theme card's hues", () => {
  it("are the person's accent, then red and green, never one twice", () => {
    expect(previewHues("peach")).toEqual(["peach", "red", "green"]);
    expect(previewHues("red")).toEqual(["red", "green", "blue"]);
    expect(previewHues("green")).toEqual(["green", "red", "blue"]);
  });
});

describe("stepChoice", () => {
  const list = ["a", "b", "c"] as const;
  it("moves along the arrows round and round, and to the ends on Home and End", () => {
    expect(stepChoice(list, "a", "ArrowRight")).toBe("b");
    expect(stepChoice(list, "b", "ArrowDown")).toBe("c");
    expect(stepChoice(list, "c", "ArrowRight")).toBe("a");
    expect(stepChoice(list, "a", "ArrowLeft")).toBe("c");
    expect(stepChoice(list, "b", "ArrowUp")).toBe("a");
    expect(stepChoice(list, "b", "Home")).toBe("a");
    expect(stepChoice(list, "a", "End")).toBe("c");
  });

  it("leaves every other key alone", () => {
    expect(stepChoice(list, "a", "Enter")).toBeNull();
    expect(stepChoice(list, "a", " ")).toBeNull();
  });
});
