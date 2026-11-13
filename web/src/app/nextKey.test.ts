import { describe, expect, it } from "vitest";
import { nextKeyAsked, walkKeyAsked, type KeyPress } from "./nextKey";

const press = (over: Partial<KeyPress> = {}): KeyPress => ({ code: "KeyN", altKey: true, metaKey: false, ctrlKey: false, shiftKey: false, target: { closest: () => null }, ...over });
// a target inside one of the given elements, as Element.closest would answer
const inside = (...tags: string[]) => ({ closest: (selector: string) => (tags.some((tag) => selector.includes(tag)) ? {} : null) });

describe("nextKeyAsked", () => {
  it("is Alt+N by key code with no other modifier, outside a dialog", () => {
    for (const mac of [true, false]) {
      expect(nextKeyAsked(press(), false, mac)).toBe(true);
      expect(nextKeyAsked(press({ code: "KeyM" }), false, mac)).toBe(false);
      expect(nextKeyAsked(press({ altKey: false }), false, mac)).toBe(false);
      expect(nextKeyAsked(press({ shiftKey: true }), false, mac)).toBe(false);
      expect(nextKeyAsked(press({ ctrlKey: true }), false, mac)).toBe(false);
      expect(nextKeyAsked(press({ metaKey: true }), false, mac)).toBe(false);
      expect(nextKeyAsked(press(), true, mac)).toBe(false);
      expect(nextKeyAsked(press({ target: inside(".xterm") }), false, mac)).toBe(false);
      expect(nextKeyAsked(press({ target: inside(".screen-term") }), false, mac)).toBe(false);
      expect(nextKeyAsked(press({ target: inside("button") }), false, mac)).toBe(true);
      expect(nextKeyAsked(press({ target: null }), false, mac)).toBe(true);
    }
  });
  it("leaves Option+N to a field on a Mac, where it types the tilde, and takes Alt+N from the composer elsewhere", () => {
    for (const field of ["textarea", "input", "[contenteditable]"]) {
      expect(nextKeyAsked(press({ target: inside(field) }), false, true)).toBe(false);
      expect(nextKeyAsked(press({ target: inside(field) }), false, false)).toBe(true);
    }
  });
});

describe("walkKeyAsked", () => {
  it("walks down on Alt+] and up on Alt+[ by key code, with Alt+N's rules", () => {
    for (const mac of [true, false]) {
      expect(walkKeyAsked(press({ code: "BracketRight" }), false, mac)).toBe(1);
      expect(walkKeyAsked(press({ code: "BracketLeft" }), false, mac)).toBe(-1);
      expect(walkKeyAsked(press(), false, mac)).toBe(0);
      expect(walkKeyAsked(press({ code: "BracketRight", altKey: false }), false, mac)).toBe(0);
      expect(walkKeyAsked(press({ code: "BracketRight", metaKey: true }), false, mac)).toBe(0);
      expect(walkKeyAsked(press({ code: "BracketLeft", shiftKey: true }), false, mac)).toBe(0);
      expect(walkKeyAsked(press({ code: "BracketRight" }), true, mac)).toBe(0);
      expect(walkKeyAsked(press({ code: "BracketRight", target: inside(".xterm") }), false, mac)).toBe(0);
    }
  });
  it("leaves Option+] to a field on a Mac, where it types a quote", () => {
    expect(walkKeyAsked(press({ code: "BracketRight", target: inside("textarea") }), false, true)).toBe(0);
    expect(walkKeyAsked(press({ code: "BracketRight", target: inside("textarea") }), false, false)).toBe(1);
  });
});
