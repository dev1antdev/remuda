import { describe, expect, it } from "vitest";
import { backTo, focusReturn, type FocusHolder } from "./dialogFocus";

// a stand-in element: its id, and the menu it sits in with the id of the button that opened it, if any
const element = (id: string, menuOf?: string): FocusHolder & { id: string } => ({
  id,
  closest: (selector: string) => (menuOf && selector === '[role="menu"]' ? { getAttribute: (name: string) => (name === "aria-labelledby" ? menuOf : null) } : null),
});

describe("focusReturn", () => {
  const trigger = element("radix-:r5:");
  const byId = (id: string) => (id === trigger.id ? trigger : null);

  it("is the element that had the focus as the dialog opened", () => {
    const field = element("field");
    expect(focusReturn(field, byId)).toBe(field);
    expect(focusReturn(null, byId)).toBeNull();
  });
  it("is the button that opened the menu for a row of it: the row leaves the page as the dialog opens", () => {
    expect(focusReturn(element("row", "radix-:r5:"), byId)).toBe(trigger);
  });
  it("is the row itself when the menu names no button that is still there", () => {
    const row = element("row", "radix-:gone:");
    expect(focusReturn(row, byId)).toBe(row);
  });
});

describe("backTo", () => {
  const button = { isConnected: true, name: "the button that had the focus before the palette" };
  it("is the opener while it is still on the page", () => {
    const field = { isConnected: true, name: "a field of the dialog under this one" };
    expect(backTo(field, button)).toBe(field);
  });
  it("is where the dialog it was opened from would have sent it once that one is gone: a dialog the palette opened", () => {
    expect(backTo({ isConnected: false, name: "the palette's field" }, button)).toBe(button);
    expect(backTo(null, button)).toBe(button);
    expect(backTo({ isConnected: false, name: "gone" }, null)).toBeNull();
  });
});
