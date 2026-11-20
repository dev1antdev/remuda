import { describe, expect, it } from "vitest";
import { kindOf } from "./kind";

describe("kindOf", () => {
  it("reads a record from before the kinds, or of a kind the page no longer has, as a prompt", () => {
    expect(kindOf({})).toBe("prompt");
    expect(kindOf({ kind: "prompt" })).toBe("prompt");
    expect(kindOf({ kind: "agent" })).toBe("agent");
    expect(kindOf({ kind: "scene" })).toBe("scene");
  });
});
