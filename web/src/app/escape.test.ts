import { describe, expect, it } from "vitest";
import { escapeIsFree, overlayOpen, type EscapePress } from "./escape";

const press = (over: Partial<EscapePress> = {}): EscapePress => ({ key: "Escape", defaultPrevented: false, target: { closest: () => null }, ...over });
const inside = (...parts: string[]) => ({ closest: (selector: string) => (parts.some((part) => selector.includes(part)) ? {} : null) });
const closed = { dialog: null, panel: null, confirm: null, file: null };

describe("escapeIsFree", () => {
  it("is an Escape on the page itself with nothing open over it", () => {
    expect(escapeIsFree(press(), false)).toBe(true);
    expect(escapeIsFree(press({ target: null }), false)).toBe(true);
    expect(escapeIsFree(press({ key: "Enter" }), false)).toBe(false);
  });
  it("leaves the Esc a menu used to close itself: Radix marks it with preventDefault, and its focus is in the menu", () => {
    expect(escapeIsFree(press({ defaultPrevented: true }), false)).toBe(false);
    expect(escapeIsFree(press({ target: inside("[role=menu]") }), false)).toBe(false);
  });
  it("leaves a field, the terminal, a dialog, the file viewer and a select their own Esc", () => {
    for (const where of ["textarea", "input", ".xterm", ".modal-scrim", ".fv-scrim", ".sel-pop"]) expect(escapeIsFree(press({ target: inside(where) }), false), where).toBe(false);
    expect(escapeIsFree(press(), true)).toBe(false);
  });
});

describe("overlayOpen", () => {
  it("counts the confirm and the file viewer, not only a dialog or a panel; a menu takes its own Esc", () => {
    expect(overlayOpen(closed)).toBe(false);
    for (const key of ["dialog", "panel", "confirm", "file"] as const) expect(overlayOpen({ ...closed, [key]: {} }), key).toBe(true);
  });
});
