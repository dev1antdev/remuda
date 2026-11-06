import { describe, expect, it } from "vitest";
import { MODE_ICON, isPermissionMode, modeTone, modeWords } from "./mode";

describe("the permission mode chip", () => {
  it("keeps only the modes the server names", () => {
    expect(isPermissionMode("acceptEdits")).toBe(true);
    expect(isPermissionMode("auto")).toBe(true);
    expect(isPermissionMode("turbo")).toBe(false);
    expect(isPermissionMode(null)).toBe(false);
  });
  it("says each mode in the footer's own words, bypass in amber", () => {
    expect(modeWords("default")).toBe("manual mode");
    expect(modeWords("acceptEdits")).toBe("accept edits");
    expect(modeWords("plan")).toBe("plan mode");
    expect(modeWords("bypass")).toBe("bypass permissions");
    expect(modeTone("bypass")).toBe("amber");
    expect(modeTone("plan")).toBeNull();
    expect(MODE_ICON.auto).toBe("sparkles");
  });
});
