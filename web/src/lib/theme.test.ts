import { describe, expect, it } from "vitest";
import css from "../styles.css?raw";
import {
  ACCENTS,
  accentName,
  accentOf,
  ansiPalette,
  flavorPrefOf,
  FLAVORS,
  FRAPPE,
  LATTE,
  MACCHIATO,
  MOCHA,
  pageColor,
  resolveFlavor,
  resolveTheme,
  terminalTheme,
  themeOf,
  themePrefOf,
  withAlpha,
  type Flavor,
} from "./theme";

// paletteBlock is the --ctp-* colours the stylesheet gives a flavour
function paletteBlock(flavor: Flavor): Record<string, string> {
  const block = new RegExp(`:root\\[data-flavor="${flavor}"\\]\\s*\\{([^}]*)\\}`).exec(css);
  if (!block) throw new Error(`no palette block for ${flavor}`);
  const out: Record<string, string> = {};
  for (const m of block[1]!.matchAll(/--ctp-([a-z0-9]+):\s*(#[0-9a-f]{6});/g)) out[m[1]!] = m[2]!;
  return out;
}

describe("resolveTheme and resolveFlavor", () => {
  it("follow the system only when the pref says so", () => {
    expect(resolveTheme("system", true)).toBe("light");
    expect(resolveTheme("system", false)).toBe("dark");
    expect(resolveTheme("dark", true)).toBe("dark");
    expect(resolveTheme("light", false)).toBe("light");
  });

  it("wear Latte on the light side and the chosen flavour on the dark one", () => {
    expect(resolveFlavor("system", "mocha", true)).toBe("latte");
    expect(resolveFlavor("system", "mocha", false)).toBe("mocha");
    expect(resolveFlavor("dark", "macchiato", true)).toBe("macchiato");
    expect(resolveFlavor("light", "frappe", false)).toBe("latte");
    expect(themeOf("latte")).toBe("light");
    expect(themeOf("macchiato")).toBe("dark");
  });
});

describe("the pref readers", () => {
  it("keep only the words they know", () => {
    expect(themePrefOf("light")).toBe("light");
    expect(themePrefOf("dark")).toBe("dark");
    expect(themePrefOf("system")).toBe("system");
    expect(themePrefOf("blue")).toBe("system");
    expect(themePrefOf(undefined)).toBe("system");
    expect(flavorPrefOf("mocha")).toBe("mocha");
    expect(flavorPrefOf("macchiato")).toBe("macchiato");
    expect(flavorPrefOf("latte")).toBe("frappe");
    expect(flavorPrefOf(undefined)).toBe("frappe");
    expect(accentOf("mauve")).toBe("mauve");
    expect(accentOf("orange")).toBe("peach");
    expect(accentOf(3)).toBe("peach");
    expect(accentName("sapphire")).toBe("Sapphire");
  });
});

describe("the palettes", () => {
  it("are Catppuccin's: every swatch a six-digit hex, each flavour on its own base", () => {
    for (const c of Object.values({ ...FRAPPE, ...LATTE, ...MACCHIATO, ...MOCHA })) expect(c).toMatch(/^#[0-9a-f]{6}$/);
    expect(FRAPPE.base).toBe("#303446");
    expect(MACCHIATO.base).toBe("#24273a");
    expect(MOCHA.base).toBe("#1e1e2e");
    expect(LATTE.base).toBe("#eff1f5");
    expect(FRAPPE.peach).toBe("#ef9f76");
    expect(LATTE.peach).toBe("#fe640b");
  });

  it("are the stylesheet's --ctp-* blocks, colour for colour", () => {
    for (const [flavor, swatches] of Object.entries(FLAVORS) as [Flavor, typeof FRAPPE][]) {
      const block = paletteBlock(flavor);
      for (const [name, hex] of Object.entries(swatches)) expect(block[name], `${flavor} ${name}`).toBe(hex);
    }
  });

  it("offer every accent in the stylesheet, peach without a pick", () => {
    for (const accent of ACCENTS) expect(css).toContain(`:root[data-accent="${accent}"] { --ctp-accent: var(--ctp-${accent}); }`);
    expect(css).toContain("--ctp-accent: var(--ctp-peach);");
    expect(css).not.toMatch(/light-dark\(/);
  });

  it("set a terminal's sixteen colours with pink for magenta and teal for cyan, and the paper is the base", () => {
    const dark = ansiPalette("frappe");
    expect(dark.colors).toHaveLength(16);
    expect(dark.colors[1]).toBe(FRAPPE.red);
    expect(dark.colors[5]).toBe(FRAPPE.pink);
    expect(dark.colors[6]).toBe(FRAPPE.teal);
    expect(dark.colors[9]).toBe(FRAPPE.red);
    expect(dark.bg).toBe(FRAPPE.base);
    expect(ansiPalette("latte").fg).toBe(LATTE.text);
    expect(ansiPalette("mocha").bg).toBe(MOCHA.base);
    const term = terminalTheme("latte");
    expect(term.background).toBe(LATTE.base);
    expect(term.cursor).toBe(LATTE.rosewater);
    expect(term.brightWhite).toBe(LATTE.subtext0);
    expect(term.selectionBackground).toBe("rgba(172, 176, 190, 0.55)");
    expect(terminalTheme("macchiato").foreground).toBe(MACCHIATO.text);
    expect(pageColor("frappe")).toBe("#303446");
    expect(pageColor("mocha")).toBe("#1e1e2e");
  });
});

describe("withAlpha", () => {
  it("turns hex into rgba", () => {
    expect(withAlpha("#ef9f76", 0.5)).toBe("rgba(239, 159, 118, 0.5)");
  });
});
