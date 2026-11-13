// The themes the page wears — ten dark and ten light, Catppuccin's four among them, each a set of colours in Catppuccin's
// 26 named slots (lib/themes/) — the mode that picks between the dark one and the light one, one of the fourteen named
// colours as the accent, and the palettes the terminal lens and the wall's screens draw with. A flavour is one theme's
// id. The stylesheet carries the same colours as --ctp-* custom properties, one block per flavour (lib/theme.test.ts
// keeps the two equal and every one readable); this is the one place the values are typed out for scripts.

import { t } from "@/lib/i18n";
import { DARK_PALETTES } from "./themes/dark";
import { LIGHT_PALETTES } from "./themes/light";
import { ACCENTS, type Accent, type Palette, type Swatches } from "./themes/palette";

export { ACCENTS, type Accent, type Swatches };

export type ThemePref = "system" | "dark" | "light";
export type Theme = "dark" | "light";
export type DarkFlavor = keyof typeof DARK_PALETTES;
export type LightFlavor = keyof typeof LIGHT_PALETTES;
export type Flavor = DarkFlavor | LightFlavor;
export const THEME_PREF_KEY = "theme";
export const DARK_FLAVOR_KEY = "theme.dark";
export const LIGHT_FLAVOR_KEY = "theme.light";
export const ACCENT_KEY = "theme.accent";

export const DEFAULT_ACCENT: Accent = "peach";
export const DEFAULT_DARK: DarkFlavor = "frappe";
export const DEFAULT_LIGHT: LightFlavor = "latte";
export const DARK_FLAVORS = Object.keys(DARK_PALETTES) as DarkFlavor[];
export const LIGHT_FLAVORS = Object.keys(LIGHT_PALETTES) as LightFlavor[];

const PALETTES: Record<Flavor, Palette> = { ...DARK_PALETTES, ...LIGHT_PALETTES };

export const FLAVORS = Object.fromEntries(Object.entries(PALETTES).map(([id, palette]) => [id, palette.swatches])) as Record<Flavor, Swatches>;

// FLAVOR_NAMES are the themes as their authors spell them
export const FLAVOR_NAMES = Object.fromEntries(Object.entries(PALETTES).map(([id, palette]) => [id, palette.name])) as Record<Flavor, string>;

const isDark = (value: unknown): value is DarkFlavor => typeof value === "string" && Object.hasOwn(DARK_PALETTES, value);
const isLight = (value: unknown): value is LightFlavor => typeof value === "string" && Object.hasOwn(LIGHT_PALETTES, value);

// isFlavor says whether a word names one of the twenty themes
export const isFlavor = (value: unknown): value is Flavor => isDark(value) || isLight(value);

// resolveTheme is the theme a pref comes to against the system's own choice
export function resolveTheme(pref: ThemePref, systemLight: boolean): Theme {
  return pref === "system" ? (systemLight ? "light" : "dark") : pref;
}

// resolveFlavor is the flavour the page wears: the light theme picked when the mode comes to light, the dark one otherwise
export function resolveFlavor(pref: ThemePref, dark: DarkFlavor, light: LightFlavor, systemLight: boolean): Flavor {
  return resolveTheme(pref, systemLight) === "light" ? light : dark;
}

// themeOf is the side of the switch a flavour sits on
export const themeOf = (flavor: Flavor): Theme => (isLight(flavor) ? "light" : "dark");

// themePrefOf keeps a stored value only when it is one of the three
export function themePrefOf(value: unknown): ThemePref {
  return value === "dark" || value === "light" ? value : "system";
}

// darkFlavorOf keeps a stored dark theme only when it is one of the ten, Frappé otherwise
export const darkFlavorOf = (value: unknown): DarkFlavor => (isDark(value) ? value : DEFAULT_DARK);

// lightFlavorOf keeps a stored light theme only when it is one of the ten, Latte otherwise
export const lightFlavorOf = (value: unknown): LightFlavor => (isLight(value) ? value : DEFAULT_LIGHT);

// accentOf keeps a stored accent only when it is one of the fourteen, peach otherwise
export function accentOf(value: unknown): Accent {
  return typeof value === "string" && (ACCENTS as readonly string[]).includes(value) ? (value as Accent) : DEFAULT_ACCENT;
}

// accentName is an accent's name with its first letter up, for a label
export const accentName = (accent: Accent): string => t(accent[0]!.toUpperCase() + accent.slice(1));

// themeChoiceName is how the mode's switch names a choice: the system's, or the dark or light theme it holds to
export function themeChoiceName(pref: ThemePref, dark: DarkFlavor, light: LightFlavor): string {
  if (pref === "dark") return t("Dark: {theme}", { theme: FLAVOR_NAMES[dark] });
  return pref === "light" ? t("Light: {theme}", { theme: FLAVOR_NAMES[light] }) : t("Follow the system");
}

export const swatches = (flavor: Flavor): Swatches => FLAVORS[flavor];

// AnsiPalette is a terminal's sixteen colours, its ink and its paper
export type AnsiPalette = { colors: string[]; fg: string; bg: string };

// ansiPalette is the terminal in a flavour: the sixteen of the theme's own terminal ports, else as Catppuccin's ports
// set them from the slots — black and white from the surfaces and subtexts, magenta as pink, cyan as teal, the bright
// half the same hues; the ink and the paper are the page's text and base either way
export function ansiPalette(flavor: Flavor): AnsiPalette {
  const c = swatches(flavor);
  const own = PALETTES[flavor].terminal?.colors;
  return {
    colors: own ?? [c.surface1, c.red, c.green, c.yellow, c.blue, c.pink, c.teal, c.subtext1, c.surface2, c.red, c.green, c.yellow, c.blue, c.pink, c.teal, c.subtext0],
    fg: c.text,
    bg: c.base,
  };
}

// terminalTheme is what xterm takes for a flavour: the palette, the theme's own cursor (rosewater without one), a
// surface for the selection
export function terminalTheme(flavor: Flavor) {
  const c = swatches(flavor);
  const p = ansiPalette(flavor);
  const [black, red, green, yellow, blue, magenta, cyan, white, brightBlack, brightRed, brightGreen, brightYellow, brightBlue, brightMagenta, brightCyan, brightWhite] = p.colors as [string, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string];
  return {
    background: p.bg,
    foreground: p.fg,
    cursor: PALETTES[flavor].terminal?.cursor ?? c.rosewater,
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

// contrast is WCAG's contrast ratio between two six-digit hex colours, 1 to 21
export function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
}

function luminance(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
