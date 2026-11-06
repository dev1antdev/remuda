import { describe, expect, it } from "vitest";
import dialogSource from "./Dialog.tsx?raw";
import { dialogTakesEscape } from "./dialogEscape";

describe("dialogTakesEscape", () => {
  it("closes the topmost dialog on an Esc nothing inside it used", () => {
    expect(dialogTakesEscape({ key: "Escape", defaultPrevented: false }, false, true)).toBe(true);
    expect(dialogTakesEscape({ key: "Enter", defaultPrevented: false }, false, true)).toBe(false);
  });
  it("leaves the Esc a tooltip or a menu inside it used to close itself, a Select's open list and a dialog under another", () => {
    expect(dialogTakesEscape({ key: "Escape", defaultPrevented: true }, false, true)).toBe(false);
    expect(dialogTakesEscape({ key: "Escape", defaultPrevented: false }, true, true)).toBe(false);
    expect(dialogTakesEscape({ key: "Escape", defaultPrevented: false }, false, false)).toBe(false);
  });
  it("is the rule useEscape asks", () => {
    expect(dialogSource).toMatch(/dialogTakesEscape\(\{ key: e\.key, defaultPrevented: e\.defaultPrevented \}/);
  });
});
