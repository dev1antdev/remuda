import { describe, expect, it } from "vitest";
import { lensToOpen, openLensOf } from "./openLens";

describe("openLens", () => {
  it("reads a stored choice and falls back to the terminal", () => {
    expect(openLensOf("chat")).toBe("chat");
    expect(openLensOf("keep")).toBe("keep");
    expect(openLensOf("changes")).toBe("terminal");
    expect(openLensOf(undefined)).toBe("terminal");
  });
  it("opens the chosen lens, or keeps the current one", () => {
    expect(lensToOpen("terminal", "chat")).toBe("terminal");
    expect(lensToOpen("chat", "terminal")).toBe("chat");
    expect(lensToOpen("keep", "changes")).toBe("changes");
  });
});
