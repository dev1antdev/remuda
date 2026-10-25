import { describe, expect, it } from "vitest";
import { ansiPalette, FRAPPE, LATTE, pageColor, resolveTheme, terminalTheme, themePrefOf, withAlpha } from "./theme";

describe("resolveTheme", () => {
  it("follows the system only when the pref says so", () => {
    expect(resolveTheme("system", true)).toBe("light");
    expect(resolveTheme("system", false)).toBe("dark");
    expect(resolveTheme("dark", true)).toBe("dark");
    expect(resolveTheme("light", false)).toBe("light");
  });
});

describe("themePrefOf", () => {
  it("keeps only the three words", () => {
    expect(themePrefOf("light")).toBe("light");
    expect(themePrefOf("dark")).toBe("dark");
    expect(themePrefOf("system")).toBe("system");
    expect(themePrefOf("blue")).toBe("system");
    expect(themePrefOf(undefined)).toBe("system");
  });
});

describe("the palettes", () => {
  it("are Catppuccin's: every swatch a six-digit hex, Frappé dark on its base, Latte light", () => {
    for (const c of Object.values({ ...FRAPPE, ...LATTE })) expect(c).toMatch(/^#[0-9a-f]{6}$/);
    expect(FRAPPE.base).toBe("#303446");
    expect(LATTE.base).toBe("#eff1f5");
    expect(FRAPPE.peach).toBe("#ef9f76");
    expect(LATTE.peach).toBe("#fe640b");
  });

  it("set a terminal's sixteen colours with pink for magenta and teal for cyan, and the paper is the base", () => {
    const dark = ansiPalette("dark");
    expect(dark.colors).toHaveLength(16);
    expect(dark.colors[1]).toBe(FRAPPE.red);
    expect(dark.colors[5]).toBe(FRAPPE.pink);
    expect(dark.colors[6]).toBe(FRAPPE.teal);
    expect(dark.colors[9]).toBe(FRAPPE.red);
    expect(dark.bg).toBe(FRAPPE.base);
    expect(ansiPalette("light").fg).toBe(LATTE.text);
    const term = terminalTheme("light");
    expect(term.background).toBe(LATTE.base);
    expect(term.cursor).toBe(LATTE.rosewater);
    expect(term.brightWhite).toBe(LATTE.subtext0);
    expect(term.selectionBackground).toBe("rgba(172, 176, 190, 0.55)");
    expect(pageColor("dark")).toBe("#303446");
  });
});

describe("withAlpha", () => {
  it("turns hex into rgba", () => {
    expect(withAlpha("#ef9f76", 0.5)).toBe("rgba(239, 159, 118, 0.5)");
  });
});
