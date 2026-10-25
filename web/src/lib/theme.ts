// The two themes, Catppuccin Frappé by night and Latte by day: which one the page wears — the system's choice unless a
// pref says — and the palettes the terminal lens and the wall's screens draw with. The stylesheet carries the same colours
// through light-dark() and the root's color-scheme; this is the one place the values are typed out for scripts.

export type ThemePref = "system" | "dark" | "light";
export type Theme = "dark" | "light";
export const THEME_PREF_KEY = "theme";

// resolveTheme is the theme a pref comes to against the system's own choice
export function resolveTheme(pref: ThemePref, systemLight: boolean): Theme {
  return pref === "system" ? (systemLight ? "light" : "dark") : pref;
}

// readThemePref keeps a stored value only when it is one of the three
export function themePrefOf(value: unknown): ThemePref {
  return value === "dark" || value === "light" ? value : "system";
}

// Swatches are Catppuccin's named colours of one flavour
export type Swatches = Record<
  "base" | "mantle" | "crust" | "surface0" | "surface1" | "surface2" | "overlay0" | "overlay1" | "overlay2" | "subtext0" | "subtext1" | "text" | "rosewater" | "flamingo" | "pink" | "mauve" | "red" | "maroon" | "peach" | "yellow" | "green" | "teal" | "sky" | "sapphire" | "blue" | "lavender",
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

export const LATTE: Swatches = {
  base: "#eff1f5", mantle: "#e6e9ef", crust: "#dce0e8",
  surface0: "#ccd0da", surface1: "#bcc0cc", surface2: "#acb0be",
  overlay0: "#9ca0b0", overlay1: "#8c8fa1", overlay2: "#7c7f93",
  subtext0: "#6c6f85", subtext1: "#5c5f77", text: "#4c4f69",
  rosewater: "#dc8a78", flamingo: "#dd7878", pink: "#ea76cb", mauve: "#8839ef", red: "#d20f39", maroon: "#e64553",
  peach: "#fe640b", yellow: "#df8e1d", green: "#40a02b", teal: "#179299", sky: "#04a5e5", sapphire: "#209fb5", blue: "#1e66f5", lavender: "#7287fd",
};

export const swatches = (theme: Theme): Swatches => (theme === "light" ? LATTE : FRAPPE);

// AnsiPalette is a terminal's sixteen colours, its ink and its paper
export type AnsiPalette = { colors: string[]; fg: string; bg: string };

// ansiPalette is the terminal in a theme as Catppuccin's own ports set it: black and white from the surfaces and subtexts,
// magenta as pink, cyan as teal, the bright half the same hues
export function ansiPalette(theme: Theme): AnsiPalette {
  const c = swatches(theme);
  return {
    colors: [c.surface1, c.red, c.green, c.yellow, c.blue, c.pink, c.teal, c.subtext1, c.surface2, c.red, c.green, c.yellow, c.blue, c.pink, c.teal, c.subtext0],
    fg: c.text,
    bg: c.base,
  };
}

// terminalTheme is what xterm takes for a theme: the palette, a rosewater cursor, a surface for the selection
export function terminalTheme(theme: Theme) {
  const c = swatches(theme);
  const p = ansiPalette(theme);
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
export const pageColor = (theme: Theme): string => swatches(theme).base;

// withAlpha is a six-digit hex colour as rgba() with the alpha given
export function withAlpha(hex: string, alpha: number): string {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}
