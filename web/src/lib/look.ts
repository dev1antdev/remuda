// The page's look beyond colour: the text size, the density and the monospace font — the prefs, what the stylesheet
// reads for each (data-text and data-density on the root, --mono on it; lib/look.test.ts keeps the numbers equal) and
// the font stacks. app/theme.ts puts them on the document.

export type TextSize = "small" | "normal" | "large" | "larger";
export type Density = "comfortable" | "compact";
export const TEXT_SIZE_KEY = "text.size";
export const DENSITY_KEY = "density";
export const MONO_KEY = "font.mono";

export const TEXT_SIZES: TextSize[] = ["small", "normal", "large", "larger"];
export const TEXT_SIZE_NAMES: Record<TextSize, string> = { small: "Small", normal: "Normal", large: "Large", larger: "Larger" };

// TEXT_SCALE is what --text-scale comes to for each size; the stylesheet's data-text rules carry the same numbers
export const TEXT_SCALE: Record<TextSize, number> = { small: 0.92, normal: 1, large: 1.1, larger: 1.2 };
export const textScaleOf = (size: TextSize): number => TEXT_SCALE[size];

// textSizeOf keeps a stored value only when it is one of the four
export function textSizeOf(value: unknown): TextSize {
  return TEXT_SIZES.includes(value as TextSize) ? (value as TextSize) : "normal";
}

// densityOf keeps a stored value only when it is one of the two
export function densityOf(value: unknown): Density {
  return value === "compact" ? "compact" : "comfortable";
}

export const DENSITY_NAMES: Record<Density, string> = { comfortable: "Comfortable", compact: "Compact" };

// SYSTEM_MONO is the stack the page and the terminal wear unless a font is picked: whatever monospace the system has
export const SYSTEM_MONO = 'ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, monospace';

// MONO_FONTS are the families on offer, each rendered from a local file when the device has it, else the system stack
export const MONO_FONTS = ["SF Mono", "Menlo", "Monaco", "JetBrains Mono", "Fira Code", "Cascadia Code", "IBM Plex Mono", "Source Code Pro", "Hack", "Ubuntu Mono", "Consolas", "Iosevka"] as const;
export type MonoFont = (typeof MONO_FONTS)[number] | "";

// monoOf keeps a stored value only when it names a font on offer; "" is the system stack
export function monoOf(value: unknown): MonoFont {
  return MONO_FONTS.includes(value as (typeof MONO_FONTS)[number]) ? (value as MonoFont) : "";
}

// monoStack is the CSS font-family for a pick: the family first, the system stack behind it
export function monoStack(font: MonoFont): string {
  return font ? `"${font}", ${SYSTEM_MONO}` : SYSTEM_MONO;
}

export const monoName = (font: MonoFont): string => font || "System monospace";

// Look is what dresses a document beside its colours
export type Look = { text: TextSize; density: Density; mono: MonoFont };
