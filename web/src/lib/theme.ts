// The flavours the page wears — Catppuccin Latte by day, Frappé, Macchiato or Mocha by night, one of the fourteen named
// colours as the accent — and the palettes the terminal lens and the wall's screens draw with. The stylesheet carries the
// same colours as --ctp-* custom properties, one block per flavour (lib/theme.test.ts keeps the two equal); this is the
// one place the values are typed out for scripts.

import { t } from "@/lib/i18n";

export type ThemePref = "system" | "dark" | "light";
export type Theme = "dark" | "light";
export type Flavor = "latte" | "frappe" | "macchiato" | "mocha";
export type DarkFlavor = Exclude<Flavor, "latte">;
export const THEME_PREF_KEY = "theme";
export const DARK_FLAVOR_KEY = "theme.dark";
export const ACCENT_KEY = "theme.accent";

export const ACCENTS = ["rosewater", "flamingo", "pink", "mauve", "red", "maroon", "peach", "yellow", "green", "teal", "sky", "sapphire", "blue", "lavender"] as const;
export type Accent = (typeof ACCENTS)[number];
export const DEFAULT_ACCENT: Accent = "peach";
export const DARK_FLAVORS: DarkFlavor[] = ["frappe", "macchiato", "mocha"];

// resolveTheme is the theme a pref comes to against the system's own choice
export function resolveTheme(pref: ThemePref, systemLight: boolean): Theme {
  return pref === "system" ? (systemLight ? "light" : "dark") : pref;
}

// resolveFlavor is the flavour the page wears: Latte when the theme is light, the chosen dark flavour otherwise
export function resolveFlavor(pref: ThemePref, dark: DarkFlavor, systemLight: boolean): Flavor {
  return resolveTheme(pref, systemLight) === "light" ? "latte" : dark;
}

// themeOf is the side of the switch a flavour sits on
export const themeOf = (flavor: Flavor): Theme => (flavor === "latte" ? "light" : "dark");

// themePrefOf keeps a stored value only when it is one of the three
export function themePrefOf(value: unknown): ThemePref {
  return value === "dark" || value === "light" ? value : "system";
}

// flavorPrefOf keeps a stored dark flavour only when it is one of the three, Frappé otherwise
export function flavorPrefOf(value: unknown): DarkFlavor {
  return value === "macchiato" || value === "mocha" ? value : "frappe";
}

// accentOf keeps a stored accent only when it is one of the fourteen, peach otherwise
export function accentOf(value: unknown): Accent {
  return typeof value === "string" && (ACCENTS as readonly string[]).includes(value) ? (value as Accent) : DEFAULT_ACCENT;
}

// Swatches are Catppuccin's named colours of one flavour
export type Swatches = Record<
  "base" | "mantle" | "crust" | "surface0" | "surface1" | "surface2" | "overlay0" | "overlay1" | "overlay2" | "subtext0" | "subtext1" | "text" | Accent,
  string
>;

export const FRAPPE: Swatches = {
  base: "#303446", mantle: "#292c3c", crust: "#232634",
  surface0: "#414559", surface1: "#51576d", surface2: "#626880",
  overlay0: "#737994", overlay1: "#838ba7", overlay2: "#949cbb",
  subtext0: "#a5adce", subtext1: "#b5bfe2", text: "#c6d0f5",
  rosewater: "#f2d5cf", flamingo: "#eebebe", pink: "#f4b8e4", mauve: "#ca9ee6", red: "#e78284", maroon: "#ea999c",
  peach: "#ef9f76", yellow: "#e5c890", green: "#a6d189", teal: "#81c8be", sky: "#99d1db", sapphire: "#85c1dc", blue: "#8caaee", lavender: "#babbf1",
};

export const MACCHIATO: Swatches = {
  base: "#24273a", mantle: "#1e2030", crust: "#181926",
  surface0: "#363a4f", surface1: "#494d64", surface2: "#5b6078",
  overlay0: "#6e738d", overlay1: "#8087a2", overlay2: "#939ab7",
  subtext0: "#a5adcb", subtext1: "#b8c0e0", text: "#cad3f5",
  rosewater: "#f4dbd6", flamingo: "#f0c6c6", pink: "#f5bde6", mauve: "#c6a0f6", red: "#ed8796", maroon: "#ee99a0",
  peach: "#f5a97f", yellow: "#eed49f", green: "#a6da95", teal: "#8bd5ca", sky: "#91d7e3", sapphire: "#7dc4e4", blue: "#8aadf4", lavender: "#b7bdf8",
};

export const MOCHA: Swatches = {
  base: "#1e1e2e", mantle: "#181825", crust: "#11111b",
  surface0: "#313244", surface1: "#45475a", surface2: "#585b70",
  overlay0: "#6c7086", overlay1: "#7f849c", overlay2: "#9399b2",
  subtext0: "#a6adc8", subtext1: "#bac2de", text: "#cdd6f4",
  rosewater: "#f5e0dc", flamingo: "#f2cdcd", pink: "#f5c2e7", mauve: "#cba6f7", red: "#f38ba8", maroon: "#eba0ac",
  peach: "#fab387", yellow: "#f9e2af", green: "#a6e3a1", teal: "#94e2d5", sky: "#89dceb", sapphire: "#74c7ec", blue: "#89b4fa", lavender: "#b4befe",
};

export const LATTE: Swatches = {
  base: "#eff1f5", mantle: "#e6e9ef", crust: "#dce0e8",
  surface0: "#ccd0da", surface1: "#bcc0cc", surface2: "#acb0be",
  overlay0: "#9ca0b0", overlay1: "#8c8fa1", overlay2: "#7c7f93",
  subtext0: "#6c6f85", subtext1: "#5c5f77", text: "#4c4f69",
  rosewater: "#dc8a78", flamingo: "#dd7878", pink: "#ea76cb", mauve: "#8839ef", red: "#d20f39", maroon: "#e64553",
  peach: "#fe640b", yellow: "#df8e1d", green: "#40a02b", teal: "#179299", sky: "#04a5e5", sapphire: "#209fb5", blue: "#1e66f5", lavender: "#7287fd",
};

export const FLAVORS: Record<Flavor, Swatches> = { latte: LATTE, frappe: FRAPPE, macchiato: MACCHIATO, mocha: MOCHA };

// FLAVOR_NAMES are the flavours as Catppuccin spells them
export const FLAVOR_NAMES: Record<Flavor, string> = { latte: "Latte", frappe: "Frappé", macchiato: "Macchiato", mocha: "Mocha" };

// accentName is an accent's name with its first letter up, for a label
export const accentName = (accent: Accent): string => t(accent[0]!.toUpperCase() + accent.slice(1));

export const swatches = (flavor: Flavor): Swatches => FLAVORS[flavor];

// AnsiPalette is a terminal's sixteen colours, its ink and its paper
export type AnsiPalette = { colors: string[]; fg: string; bg: string };

// ansiPalette is the terminal in a flavour as Catppuccin's own ports set it: black and white from the surfaces and
// subtexts, magenta as pink, cyan as teal, the bright half the same hues
export function ansiPalette(flavor: Flavor): AnsiPalette {
  const c = swatches(flavor);
  return {
    colors: [c.surface1, c.red, c.green, c.yellow, c.blue, c.pink, c.teal, c.subtext1, c.surface2, c.red, c.green, c.yellow, c.blue, c.pink, c.teal, c.subtext0],
    fg: c.text,
    bg: c.base,
  };
}

// terminalTheme is what xterm takes for a flavour: the palette, a rosewater cursor, a surface for the selection
export function terminalTheme(flavor: Flavor) {
  const c = swatches(flavor);
  const p = ansiPalette(flavor);
  const [black, red, green, yellow, blue, magenta, cyan, white, brightBlack, brightRed, brightGreen, brightYellow, brightBlue, brightMagenta, brightCyan, brightWhite] = p.colors as [string, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string];
  return {
    background: p.bg,
    foreground: p.fg,
    cursor: c.rosewater,
    cursorAccent: c.base,
    selectionBackground: withAlpha(c.surface2, 0.55),
    black, red, green, yellow, blue, magenta, cyan, white,
    brightBlack, brightRed, brightGreen, brightYellow, brightBlue, brightMagenta, brightCyan, brightWhite,
  };
}

// pageColor is the colour the browser's own chrome takes beside the page, <meta name="theme-color">
export const pageColor = (flavor: Flavor): string => swatches(flavor).base;

// withAlpha is a six-digit hex colour as rgba() with the alpha given
export function withAlpha(hex: string, alpha: number): string {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}
