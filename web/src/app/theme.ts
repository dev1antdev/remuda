// The flavour the page wears: a theme pref (system, dark, light) resolved against the system's choice, the dark flavour
// pref (Frappé, Macchiato, Mocha) and the accent pref, set as data-theme, data-flavor and data-accent on the root of this
// document — and of the pop-out's — with the browser's chrome told through theme-color. Started once from main.tsx before
// the first render; the hooks follow every change of a pref or of the system.

import { readPref, watchPref, writePref } from "@/lib/prefs";
import { createStore } from "@/lib/store";
import {
  ACCENT_KEY,
  accentOf,
  DARK_FLAVOR_KEY,
  flavorPrefOf,
  pageColor,
  resolveFlavor,
  THEME_PREF_KEY,
  themeOf,
  themePrefOf,
  type Accent,
  type DarkFlavor,
  type Flavor,
  type Theme,
  type ThemePref,
} from "@/lib/theme";

const media = typeof matchMedia === "function" ? matchMedia("(prefers-color-scheme: light)") : null;

export const readThemePref = (): ThemePref => themePrefOf(readPref<string>(THEME_PREF_KEY, "system"));
export const readDarkFlavorPref = (): DarkFlavor => flavorPrefOf(readPref<string>(DARK_FLAVOR_KEY, "frappe"));
export const readAccentPref = (): Accent => accentOf(readPref<string>(ACCENT_KEY, "peach"));

type Dress = { theme: Theme; flavor: Flavor; accent: Accent; pref: ThemePref; darkPref: DarkFlavor };

function resolved(): Dress {
  const pref = readThemePref();
  const darkPref = readDarkFlavorPref();
  const flavor = resolveFlavor(pref, darkPref, !!media?.matches);
  return { theme: themeOf(flavor), flavor, accent: readAccentPref(), pref, darkPref };
}

const store = createStore<Dress>(resolved());

// startTheme dresses the document now and again whenever a pref or the system changes
export function startTheme(): void {
  apply();
  media?.addEventListener("change", apply);
  watchPref(THEME_PREF_KEY, apply);
  watchPref(DARK_FLAVOR_KEY, apply);
  watchPref(ACCENT_KEY, apply);
}

function apply() {
  const dress = resolved();
  store.set(dress);
  dressDocument(document, dress.flavor, dress.accent);
}

// dressDocument puts the flavour and the accent on a document's root: the stylesheet switches on the data attributes,
// the browser on theme-color
export function dressDocument(doc: Document, flavor: Flavor, accent: Accent): void {
  doc.documentElement.dataset.theme = themeOf(flavor);
  doc.documentElement.dataset.flavor = flavor;
  doc.documentElement.dataset.accent = accent;
  doc.querySelector('meta[name="theme-color"]')?.setAttribute("content", pageColor(flavor));
}

export const useTheme = (): Theme => store.use((s) => s.theme);
export const useFlavor = (): Flavor => store.use((s) => s.flavor);
export const useAccent = (): Accent => store.use((s) => s.accent);
export const useThemePref = (): ThemePref => store.use((s) => s.pref);
export const useDarkFlavorPref = (): DarkFlavor => store.use((s) => s.darkPref);
export const currentTheme = (): Theme => store.get().theme;
export const currentFlavor = (): Flavor => store.get().flavor;
export const setThemePref = (pref: ThemePref): void => writePref(THEME_PREF_KEY, pref);
export const setDarkFlavorPref = (flavor: DarkFlavor): void => writePref(DARK_FLAVOR_KEY, flavor);
export const setAccentPref = (accent: Accent): void => writePref(ACCENT_KEY, accent);
