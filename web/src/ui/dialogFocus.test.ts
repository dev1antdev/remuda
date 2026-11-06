import { describe, expect, it } from "vitest";
import { focusReturn, type FocusHolder } from "./dialogFocus";

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
