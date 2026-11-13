// Legible colours on tinted rows, after gitview: a syntax colour that falls under MIN_CONTRAST against what is really
// behind it — the row's tint over the ground it sits on, the changed words' mark over that — is lifted toward the theme's
// text in twentieths, just enough; a colour that reads keeps its hue. Worked out once per theme and kept, as a small
// stylesheet of the rows that need it (app/theme.ts puts it on the page).

import { contrast, swatches, type Flavor } from "./theme";
import type { Swatches } from "./themes/palette";

export const MIN_CONTRAST = 3.5;

type Slot = keyof Swatches;

// TOKEN_SLOTS is the swatch each token kind wears, as styles.css's .hl-* rules give them
export const TOKEN_SLOTS = {
  comment: "overlay1",
  string: "green",
  number: "peach",
  constant: "peach",
  keyword: "mauve",
  builtin: "red",
  type: "yellow",
  function: "blue",
  operator: "sky",
  tag: "blue",
  attr: "yellow",
  key: "lavender",
} as const satisfies Record<string, Slot>;

// TINT and MARK are the shares styles.css mixes a row's colour in at: --green-soft 18 %, --green-ring 50 %
const TINT = 0.18;
const MARK = 0.5;
// PUNCT is the opacity a tinted row's punctuation keeps of the row's own colour
const PUNCT = 0.75;

// TintedRow is one kind of tinted row: where it sits, its tint and its own ink
export type TintedRow = { selector: string; ground: Slot; tint: Slot; ink: Slot; punct: boolean };

// TINTED_ROWS are the rows with a tint: the diffs on the page's base (the file viewer), the Changes lens's and an edit's
// on the raised surface, a conflict's two sides
export const TINTED_ROWS: TintedRow[] = [
  { selector: ".fv-diff .add", ground: "base", tint: "green", ink: "green", punct: true },
  { selector: ".fv-diff .del", ground: "base", tint: "red", ink: "red", punct: true },
  { selector: ".review-file .fv-diff .add", ground: "mantle", tint: "green", ink: "green", punct: true },
  { selector: ".review-file .fv-diff .del", ground: "mantle", tint: "red", ink: "red", punct: true },
  { selector: ".edit-diff.fv-diff .add", ground: "mantle", tint: "green", ink: "green", punct: true },
  { selector: ".edit-diff.fv-diff .del", ground: "mantle", tint: "red", ink: "red", punct: true },
  { selector: ".conflict-view .yours", ground: "mantle", tint: "blue", ink: "text", punct: false },
  { selector: ".conflict-view .incoming", ground: "mantle", tint: "green", ink: "text", punct: false },
];

type Rgb = [number, number, number];

const rgbOf = (hex: string): Rgb => {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
};
const hexOf = (rgb: Rgb): string => `#${rgb.map((v) => Math.round(v).toString(16).padStart(2, "0")).join("")}`;

// mix is `share` of top over under, as color-mix in srgb draws a translucent colour over what is behind it
export const mix = (top: string, under: string, share: number): string => {
  const a = rgbOf(top);
  const b = rgbOf(under);
  return hexOf([0, 1, 2].map((i) => a[i]! * share + b[i]! * (1 - share)) as Rgb);
};

// lift is fg moved toward ink in twentieths until it reads at MIN_CONTRAST on bg, fg itself when it already does
export function lift(fg: string, bg: string, ink: string): string {
  if (contrast(fg, bg) >= MIN_CONTRAST) return fg;
  let lifted = fg;
  for (let step = 1; step <= 20 && contrast(lifted, bg) < MIN_CONTRAST; step++) lifted = mix(ink, fg, step / 20);
  return lifted;
}

// legibleRules is the stylesheet a theme needs: a rule for every row colour and token colour that had to be lifted on
// its row's tint or on the changed words' mark there, none for one that reads as it is
export function legibleRules(flavor: Flavor): string {
  const colours = swatches(flavor);
  const rules: string[] = [];
  const rule = (selector: string, original: string, drawn: string, extra = "") => {
    if (drawn !== original || extra) rules.push(`${selector} { color: ${drawn};${extra} }`);
  };
  for (const row of TINTED_ROWS) {
    const tinted = mix(colours[row.tint], colours[row.ground], TINT);
    const marked = mix(colours[row.tint], tinted, MARK);
    const ink = colours[row.ink];
    for (const [ground, prefix] of [
      [tinted, row.selector],
      [marked, `${row.selector} .fv-mark`],
    ] as const) {
      rule(prefix, ink, lift(ink, ground, colours.text));
      for (const [kind, slot] of Object.entries(TOKEN_SLOTS)) rule(`${prefix} .hl-${kind}`, colours[slot], lift(colours[slot], ground, colours.text));
      if (row.punct) {
        const punct = mix(ink, ground, PUNCT);
        const drawn = lift(punct, ground, colours.text);
        if (drawn !== punct) rule(`${prefix} .hl-punct`, punct, drawn, " opacity: 1;");
      }
    }
  }
  return rules.join("\n");
}

// legibleCache keeps each theme's stylesheet once worked out
const legibleCache = new Map<Flavor, string>();
export function legibleFor(flavor: Flavor): string {
  let rules = legibleCache.get(flavor);
  if (rules === undefined) legibleCache.set(flavor, (rules = legibleRules(flavor)));
  return rules;
}

// drawnOn is what a colour reads as on a tinted row once lifted, for the tests and anyone asking
export const drawnOn = (flavor: Flavor, row: TintedRow, slot: Slot): { colour: string; ground: string } => {
  const colours = swatches(flavor);
  const ground = mix(colours[row.tint], colours[row.ground], TINT);
  return { colour: lift(colours[slot], ground, colours.text), ground };
};
