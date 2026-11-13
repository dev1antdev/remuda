// The look the page wears: a theme pref (system, dark, light) resolved against the system's choice, the dark theme and
// the light theme it picks between (any of the ten each, Frappé and Latte unless set) and the accent pref, set as
// data-theme, data-flavor and data-accent on the root of this document — and of the pop-out's — with the browser's chrome
// told through theme-color; and beside the colours the text size, the density and the monospace font (lib/look.ts) as
// data-text, data-density and --mono on the same root. Started once from main.tsx before the first render; the hooks
// follow every change of a pref or of the system.

import { legibleFor } from "@/lib/legible";
import { DENSITY_KEY, densityOf, MONO_KEY, monoOf, monoStack, TEXT_SIZE_KEY, textSizeOf, type Density, type Look, type MonoFont, type TextSize } from "@/lib/look";
import { readPref, watchPref, writePref } from "@/lib/prefs";
import { createStore } from "@/lib/store";
import {
  ACCENT_KEY,
  accentOf,
  DARK_FLAVOR_KEY,
  darkFlavorOf,
  DEFAULT_DARK,
  DEFAULT_LIGHT,
  LIGHT_FLAVOR_KEY,
  lightFlavorOf,
  pageColor,
  resolveFlavor,
  THEME_PREF_KEY,
  themeOf,
  themePrefOf,
  type Accent,
  type DarkFlavor,
  type Flavor,
  type LightFlavor,
  type Theme,
  type ThemePref,
} from "@/lib/theme";

const media = typeof matchMedia === "function" ? matchMedia("(prefers-color-scheme: light)") : null;

export const readThemePref = (): ThemePref => themePrefOf(readPref<string>(THEME_PREF_KEY, "system"));
export const readDarkFlavorPref = (): DarkFlavor => darkFlavorOf(readPref<string>(DARK_FLAVOR_KEY, DEFAULT_DARK));
export const readLightFlavorPref = (): LightFlavor => lightFlavorOf(readPref<string>(LIGHT_FLAVOR_KEY, DEFAULT_LIGHT));
export const readAccentPref = (): Accent => accentOf(readPref<string>(ACCENT_KEY, "peach"));
export const readTextSizePref = (): TextSize => textSizeOf(readPref<string>(TEXT_SIZE_KEY, "normal"));
export const readDensityPref = (): Density => densityOf(readPref<string>(DENSITY_KEY, "comfortable"));
export const readMonoPref = (): MonoFont => monoOf(readPref<string>(MONO_KEY, ""));

type Dress = Look & { theme: Theme; flavor: Flavor; accent: Accent; pref: ThemePref; darkPref: DarkFlavor; lightPref: LightFlavor };

function resolved(): Dress {
  const pref = readThemePref();
  const darkPref = readDarkFlavorPref();
  const lightPref = readLightFlavorPref();
  const flavor = resolveFlavor(pref, darkPref, lightPref, !!media?.matches);
  return { theme: themeOf(flavor), flavor, accent: readAccentPref(), pref, darkPref, lightPref, text: readTextSizePref(), density: readDensityPref(), mono: readMonoPref() };
}

const store = createStore<Dress>(resolved());

// the other documents of ours that wear the page's look: the pop-out's
const others = new Set<Document>();

// startTheme dresses the document now and again whenever a pref or the system changes
export function startTheme(): void {
  apply();
  media?.addEventListener("change", apply);
  for (const key of [THEME_PREF_KEY, DARK_FLAVOR_KEY, LIGHT_FLAVOR_KEY, ACCENT_KEY, TEXT_SIZE_KEY, DENSITY_KEY, MONO_KEY]) watchPref(key, apply);
}

// dressAlso dresses another document of ours now and on every change after, until the return lets it go
export function dressAlso(doc: Document): () => void {
  others.add(doc);
  dress(doc, store.get());
  return () => {
    others.delete(doc);
  };
}

function apply() {
  const look = resolved();
  store.set(look);
  dress(document, look);
  for (const doc of others) dress(doc, look);
}

function dress(doc: Document, look: Dress) {
  dressDocument(doc, look.flavor, look.accent);
  dressLook(doc, look);
}

// dressDocument puts the flavour, its side and the accent on a document's root: the stylesheet switches on the data
// attributes, the browser on theme-color
export function dressDocument(doc: Document, flavor: Flavor, accent: Accent): void {
  doc.documentElement.dataset.theme = themeOf(flavor);
  doc.documentElement.dataset.flavor = flavor;
  doc.documentElement.dataset.accent = accent;
  doc.querySelector('meta[name="theme-color"]')?.setAttribute("content", pageColor(flavor));
  dressLegible(doc, flavor);
}

// dressLegible puts the theme's lifted colours for tinted diff rows (lib/legible.ts) on the document, after its stylesheets
function dressLegible(doc: Document, flavor: Flavor): void {
  let sheet = doc.getElementById(LEGIBLE_ID);
  if (!sheet) {
    sheet = doc.createElement("style");
    sheet.id = LEGIBLE_ID;
    doc.head.appendChild(sheet);
  }
  const rules = legibleFor(flavor);
  if (sheet.textContent !== rules) sheet.textContent = rules;
}

const LEGIBLE_ID = "remuda-legible";

// dressLook puts the text size, the density and the monospace stack on a document's root
export function dressLook(doc: Document, look: Look): void {
  const root = doc.documentElement;
  if (look.text === "normal") delete root.dataset.text;
  else root.dataset.text = look.text;
  if (look.density === "comfortable") delete root.dataset.density;
  else root.dataset.density = look.density;
  if (look.mono) root.style.setProperty("--mono", monoStack(look.mono));
  else root.style.removeProperty("--mono");
}

export const useFlavor = (): Flavor => store.use((s) => s.flavor);
export const useAccent = (): Accent => store.use((s) => s.accent);
export const useThemePref = (): ThemePref => store.use((s) => s.pref);
export const useDarkFlavorPref = (): DarkFlavor => store.use((s) => s.darkPref);
export const useLightFlavorPref = (): LightFlavor => store.use((s) => s.lightPref);
export const useTextSize = (): TextSize => store.use((s) => s.text);
export const useDensity = (): Density => store.use((s) => s.density);
export const useMono = (): MonoFont => store.use((s) => s.mono);
export const currentTheme = (): Theme => store.get().theme;
export const currentFlavor = (): Flavor => store.get().flavor;
export const currentLook = (): Look => {
  const { text, density, mono } = store.get();
  return { text, density, mono };
};
export const setThemePref = (pref: ThemePref): void => writePref(THEME_PREF_KEY, pref);
export const setDarkFlavorPref = (flavor: DarkFlavor): void => writePref(DARK_FLAVOR_KEY, flavor);
export const setLightFlavorPref = (flavor: LightFlavor): void => writePref(LIGHT_FLAVOR_KEY, flavor);
export const setAccentPref = (accent: Accent): void => writePref(ACCENT_KEY, accent);
export const setTextSizePref = (size: TextSize): void => writePref(TEXT_SIZE_KEY, size);
export const setDensityPref = (density: Density): void => writePref(DENSITY_KEY, density);
export const setMonoPref = (font: MonoFont): void => writePref(MONO_KEY, font);
