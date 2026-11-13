// What a theme is to the page: a name, its colours in Catppuccin's 26 named slots, and the terminal sixteen of its own
// ports when it has them. lib/theme.ts reads the twenty of them (themes/dark.ts, themes/light.ts).

export const ACCENTS = ["rosewater", "flamingo", "pink", "mauve", "red", "maroon", "peach", "yellow", "green", "teal", "sky", "sapphire", "blue", "lavender"] as const;
export type Accent = (typeof ACCENTS)[number];

// Swatches are a theme's colours in Catppuccin's slots: crust < mantle < base the backgrounds, surface0–2 and overlay0–2
// the steps toward the text, subtext0–1 and text the inks, and the fourteen accents
export type Swatches = Record<
  "base" | "mantle" | "crust" | "surface0" | "surface1" | "surface2" | "overlay0" | "overlay1" | "overlay2" | "subtext0" | "subtext1" | "text" | Accent,
  string
>;

// TerminalColours are a terminal port's sixteen ANSI colours, black to bright white, and its cursor
export type TerminalColours = { colors: string[]; cursor: string };

export type Palette = { name: string; swatches: Swatches; terminal?: TerminalColours };
