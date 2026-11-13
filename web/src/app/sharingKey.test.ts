import { describe, expect, it } from "vitest";
import { sharingKeyAsked } from "./sharingKey";

const press = (over: Partial<Parameters<typeof sharingKeyAsked>[0]> = {}) => ({ code: "KeyH", altKey: true, metaKey: false, ctrlKey: false, shiftKey: false, target: null, ...over });
const inside = (selector: string) => ({ closest: (s: string) => (s.includes(selector) ? {} : null) });

describe("sharingKeyAsked", () => {
  it("is Alt+H by key code and nothing else", () => {
    expect(sharingKeyAsked(press(), false, false)).toBe(true);
    expect(sharingKeyAsked(press({ code: "KeyN" }), false, false)).toBe(false);
    expect(sharingKeyAsked(press({ altKey: false }), false, false)).toBe(false);
    expect(sharingKeyAsked(press({ shiftKey: true }), false, false)).toBe(false);
    expect(sharingKeyAsked(press({ metaKey: true }), false, false)).toBe(false);
  });

  it("leaves the terminal, a dialog and a Mac's fields alone", () => {
    expect(sharingKeyAsked(press({ target: inside(".xterm") }), false, false)).toBe(false);
    expect(sharingKeyAsked(press({ target: inside(".screen-term") }), false, false)).toBe(false);
    expect(sharingKeyAsked(press(), true, false)).toBe(false);
    expect(sharingKeyAsked(press({ target: inside("textarea") }), false, true)).toBe(false);
    expect(sharingKeyAsked(press({ target: inside("textarea") }), false, false)).toBe(true);
  });
});
