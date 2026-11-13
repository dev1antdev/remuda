import { afterEach, describe, expect, it } from "vitest";
import css from "../styles.css?raw";
import { setLang } from "./i18n";
import {
  ACCENTS,
  accentName,
  accentOf,
  ansiPalette,
  contrast,
  DARK_FLAVORS,
  darkFlavorOf,
  FLAVOR_NAMES,
  FLAVORS,
  isFlavor,
  LIGHT_FLAVORS,
  lightFlavorOf,
  pageColor,
  resolveFlavor,
  resolveTheme,
  terminalTheme,
  themeChoiceName,
  themeOf,
  themePrefOf,
  withAlpha,
  type Flavor,
} from "./theme";

// paletteBlock is the --ctp-* values the stylesheet gives a theme, a var(--ctp-x) read as the block's own x
function paletteBlock(flavor: Flavor): Record<string, string> {
  const block = new RegExp(`:root\\[data-flavor="${flavor}"\\]\\s*\\{([^}]*)\\}`).exec(css);
  if (!block) throw new Error(`no palette block for ${flavor}`);
  const out: Record<string, string> = {};
  for (const m of block[1]!.matchAll(/--ctp-([a-z0-9-]+):\s*(#[0-9a-f]{6});/g)) out[m[1]!] = m[2]!;
  for (const m of block[1]!.matchAll(/--ctp-([a-z0-9-]+):\s*var\(--ctp-([a-z0-9-]+)\);/g)) out[m[1]!] = out[m[2]!] ?? `unresolved ${m[2]}`;
  return out;
}

const ALL = [...DARK_FLAVORS, ...LIGHT_FLAVORS];

describe("the themes", () => {
  it("are ten dark and ten light, Catppuccin's four among them, each on its own side", () => {
    expect(DARK_FLAVORS).toEqual(["frappe", "macchiato", "mocha", "dracula", "nord", "gruvbox-dark", "tokyo-night", "rose-pine", "one-dark", "kanagawa"]);
    expect(LIGHT_FLAVORS).toEqual(["latte", "solarized-light", "gruvbox-light", "rose-pine-dawn", "tokyo-night-day", "github-light", "one-light", "everforest-light", "ayu-light", "flexoki-light"]);
    for (const f of DARK_FLAVORS) expect(themeOf(f), f).toBe("dark");
    for (const f of LIGHT_FLAVORS) expect(themeOf(f), f).toBe("light");
    expect(FLAVOR_NAMES["rose-pine-dawn"]).toBe("Rosé Pine Dawn");
    expect(isFlavor("nord")).toBe(true);
    expect(isFlavor("toString")).toBe(false);
  });

  it("are every slot a six-digit hex, Catppuccin's dark flavours its own values", () => {
    for (const f of ALL) for (const [slot, hex] of Object.entries(FLAVORS[f])) expect(hex, `${f} ${slot}`).toMatch(/^#[0-9a-f]{6}$/);
    expect(FLAVORS.frappe.base).toBe("#303446");
    expect(FLAVORS.macchiato.base).toBe("#24273a");
    expect(FLAVORS.mocha.base).toBe("#1e1e2e");
    expect(FLAVORS.frappe.peach).toBe("#ef9f76");
    expect(FLAVORS.latte.base).toBe("#eff1f5");
    expect(FLAVORS.latte.peach).toBe("#fe640b");
    expect(FLAVORS.dracula.base).toBe("#282a36");
    expect(FLAVORS.nord.base).toBe("#2e3440");
    expect(FLAVORS["solarized-light"].base).toBe("#fdf6e3");
  });
});

describe("resolveTheme and resolveFlavor", () => {
  it("follow the system only when the pref says so", () => {
    expect(resolveTheme("system", true)).toBe("light");
    expect(resolveTheme("system", false)).toBe("dark");
    expect(resolveTheme("dark", true)).toBe("dark");
    expect(resolveTheme("light", false)).toBe("light");
  });

  it("wear the light theme picked on the light side and the dark one on the dark side", () => {
    expect(resolveFlavor("system", "mocha", "latte", true)).toBe("latte");
    expect(resolveFlavor("system", "mocha", "github-light", true)).toBe("github-light");
    expect(resolveFlavor("system", "nord", "latte", false)).toBe("nord");
    expect(resolveFlavor("dark", "macchiato", "ayu-light", true)).toBe("macchiato");
    expect(resolveFlavor("light", "frappe", "flexoki-light", false)).toBe("flexoki-light");
  });
});

describe("the pref readers", () => {
  it("keep only the words they know, so a value from before still reads and an unknown one is the default", () => {
    expect(themePrefOf("light")).toBe("light");
    expect(themePrefOf("dark")).toBe("dark");
    expect(themePrefOf("system")).toBe("system");
    expect(themePrefOf("blue")).toBe("system");
    expect(themePrefOf(undefined)).toBe("system");
    expect(darkFlavorOf("mocha")).toBe("mocha");
    expect(darkFlavorOf("macchiato")).toBe("macchiato");
    expect(darkFlavorOf("frappe")).toBe("frappe");
    expect(darkFlavorOf("tokyo-night")).toBe("tokyo-night");
    expect(darkFlavorOf("latte")).toBe("frappe");
    expect(darkFlavorOf("monokai")).toBe("frappe");
    expect(darkFlavorOf(undefined)).toBe("frappe");
    expect(lightFlavorOf("latte")).toBe("latte");
    expect(lightFlavorOf("rose-pine-dawn")).toBe("rose-pine-dawn");
    expect(lightFlavorOf("mocha")).toBe("latte");
    expect(lightFlavorOf("hasOwnProperty")).toBe("latte");
    expect(lightFlavorOf(7)).toBe("latte");
    expect(accentOf("mauve")).toBe("mauve");
    expect(accentOf("orange")).toBe("peach");
    expect(accentOf(3)).toBe("peach");
    expect(accentName("sapphire")).toBe("Sapphire");
  });
});

describe("the stylesheet's palettes", () => {
  it("are the themes' swatches, colour for colour", () => {
    for (const flavor of ALL) {
      const block = paletteBlock(flavor);
      for (const [name, hex] of Object.entries(FLAVORS[flavor])) expect(block[name], `${flavor} ${name}`).toBe(hex);
    }
  });

  it("give every theme its two inks and say which each accent wears", () => {
    for (const flavor of ALL) {
      const block = paletteBlock(flavor);
      expect(block["ink-dark"], flavor).toMatch(/^#[0-9a-f]{6}$/);
      expect(block["ink-light"], flavor).toMatch(/^#[0-9a-f]{6}$/);
      for (const accent of ACCENTS) expect([block["ink-dark"], block["ink-light"]], `${flavor} on-${accent}`).toContain(block[`on-${accent}`]);
    }
  });

  it("offer every accent with the ink that reads on it, peach without a pick, and key the light inks on the side", () => {
    for (const accent of ACCENTS) expect(css).toContain(`:root[data-accent="${accent}"] { --ctp-accent: var(--ctp-${accent}); --ctp-on-accent: var(--ctp-on-${accent}); }`);
    expect(css).toContain("--ctp-accent: var(--ctp-peach);");
    expect(css).toContain("--ctp-on-amber: var(--ctp-on-yellow);");
    expect(css).toMatch(/:root\[data-theme="light"\] \{[^}]*color-scheme: light;/);
    expect(css).not.toMatch(/data-flavor="latte"\]\s+[.#\w]/);
    expect(css).not.toMatch(/light-dark\(/);
  });
});

describe("every theme reads", () => {
  it.each(ALL)("%s: text at 7:1 on the base and the mantle, dim text at 4.5:1, the faint at 3:1, every accent's ink at 4.5:1", (flavor) => {
    const c = FLAVORS[flavor];
    const block = paletteBlock(flavor);
    const ratios: Record<string, number> = {
      "text/base": contrast(c.text, c.base),
      "text/mantle": contrast(c.text, c.mantle),
      "subtext0/base": contrast(c.subtext0, c.base),
      "overlay1/base": contrast(c.overlay1, c.base),
    };
    const floors: Record<string, number> = { "text/base": 7, "text/mantle": 7, "subtext0/base": 4.5, "overlay1/base": 3 };
    for (const accent of ACCENTS) {
      ratios[`on-${accent}`] = contrast(block[`on-${accent}`]!, c[accent]);
      floors[`on-${accent}`] = 4.5;
    }
    const short = Object.entries(ratios).filter(([pair, ratio]) => ratio < floors[pair]!).map(([pair, ratio]) => `${pair} ${ratio.toFixed(2)}`);
    expect(short).toEqual([]);
  });

  it("measures contrast as WCAG does", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrast("#ffffff", "#ffffff")).toBe(1);
    expect(contrast("#4c4f69", "#eff1f5")).toBeCloseTo(7.06, 2);
  });
});

describe("the terminal's palettes", () => {
  it("are Catppuccin's ports for Catppuccin, with pink for magenta and teal for cyan, the paper the base", () => {
    const dark = ansiPalette("frappe");
    expect(dark.colors).toHaveLength(16);
    expect(dark.colors[1]).toBe(FLAVORS.frappe.red);
    expect(dark.colors[5]).toBe(FLAVORS.frappe.pink);
    expect(dark.colors[6]).toBe(FLAVORS.frappe.teal);
    expect(dark.colors[9]).toBe(FLAVORS.frappe.red);
    expect(dark.bg).toBe(FLAVORS.frappe.base);
    expect(ansiPalette("latte").fg).toBe(FLAVORS.latte.text);
    expect(ansiPalette("mocha").bg).toBe(FLAVORS.mocha.base);
    const term = terminalTheme("latte");
    expect(term.background).toBe(FLAVORS.latte.base);
    expect(term.cursor).toBe(FLAVORS.latte.rosewater);
    expect(term.brightWhite).toBe(FLAVORS.latte.subtext0);
    expect(term.selectionBackground).toBe("rgba(172, 176, 190, 0.55)");
    expect(terminalTheme("macchiato").foreground).toBe(FLAVORS.macchiato.text);
    expect(pageColor("frappe")).toBe("#303446");
    expect(pageColor("mocha")).toBe("#1e1e2e");
  });

  it("are each theme's own sixteen where its ports give them, worked out from the slots where they do not", () => {
    expect(ansiPalette("dracula").colors).toEqual(["#21222c", "#ff5555", "#50fa7b", "#f1fa8c", "#bd93f9", "#ff79c6", "#8be9fd", "#f8f8f2", "#6272a4", "#ff6e6e", "#69ff94", "#ffffa5", "#d6acff", "#ff92df", "#a4ffff", "#ffffff"]);
    expect(ansiPalette("solarized-light").colors[0]).toBe("#073642");
    expect(ansiPalette("one-dark").colors[5]).toBe(FLAVORS["one-dark"].pink);
    expect(terminalTheme("dracula").cursor).toBe("#f8f8f2");
    expect(terminalTheme("nord").blue).toBe("#81a1c1");
    for (const f of ALL) expect(ansiPalette(f).colors, f).toHaveLength(16);
    for (const f of ALL) for (const hex of ansiPalette(f).colors) expect(hex, f).toMatch(/^#[0-9a-f]{6}$/);
    expect(pageColor("github-light")).toBe("#ffffff");
  });
});

describe("withAlpha", () => {
  it("turns hex into rgba", () => {
    expect(withAlpha("#ef9f76", 0.5)).toBe("rgba(239, 159, 118, 0.5)");
  });
});

describe("themeChoiceName", () => {
  afterEach(() => setLang("en"));
  it("names the mode's choices with the themes they hold to, in the page's language", () => {
    expect([themeChoiceName("system", "mocha", "latte"), themeChoiceName("dark", "nord", "latte"), themeChoiceName("light", "nord", "ayu-light")]).toEqual(["Follow the system", "Dark: Nord", "Light: Ayu Light"]);
    setLang("ru");
    expect([themeChoiceName("system", "frappe", "latte"), themeChoiceName("dark", "frappe", "latte"), themeChoiceName("light", "frappe", "latte")]).toEqual(["Как в системе", "Тёмная: Catppuccin Frappé", "Светлая: Catppuccin Latte"]);
  });
});

describe("the stylesheet's colours", () => {
  it("are tokens everywhere but the palette's own --ctp-* lines: a literal colour would not follow the theme", () => {
    const literal = /#[0-9a-fA-F]{3,8}\b|\b(?:rgb|hsl)a?\(\s*\d/;
    const lines = css.replace(/\/\*[\s\S]*?\*\//g, (comment) => comment.replace(/[^\n]/g, " ")).split("\n");
    const strays = lines.map((line, i) => [i + 1, line.trim()] as const).filter(([, line]) => literal.test(line) && !line.startsWith("--ctp-"));
    expect(strays).toEqual([]);
  });
});
