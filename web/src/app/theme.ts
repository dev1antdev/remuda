// The theme the page wears: a pref (system, dark, light) resolved against the system's choice, set as data-theme on the
// root of this document — and of the pop-out's — with the browser's chrome told through theme-color. Started once from
// main.tsx before the first render; the hooks follow every change of the pref or of the system.

import { readPref, watchPref, writePref } from "@/lib/prefs";
import { createStore } from "@/lib/store";
import { pageColor, resolveTheme, THEME_PREF_KEY, themePrefOf, type Theme, type ThemePref } from "@/lib/theme";

const media = typeof matchMedia === "function" ? matchMedia("(prefers-color-scheme: light)") : null;

export const readThemePref = (): ThemePref => themePrefOf(readPref<string>(THEME_PREF_KEY, "system"));

const resolved = () => resolveTheme(readThemePref(), !!media?.matches);

const store = createStore<{ theme: Theme; pref: ThemePref }>({ theme: resolved(), pref: readThemePref() });

// startTheme dresses the document now and again whenever the pref or the system changes
export function startTheme(): void {
  apply();
  media?.addEventListener("change", apply);
  watchPref(THEME_PREF_KEY, apply);
}

function apply() {
  const theme = resolved();
  store.set({ theme, pref: readThemePref() });
  dressDocument(document, theme);
}

// dressDocument puts the theme on a document's root: the stylesheet switches on data-theme, the browser on theme-color
export function dressDocument(doc: Document, theme: Theme): void {
  doc.documentElement.dataset.theme = theme;
  doc.querySelector('meta[name="theme-color"]')?.setAttribute("content", pageColor(theme));
}

export const useTheme = (): Theme => store.use((s) => s.theme);
export const useThemePref = (): ThemePref => store.use((s) => s.pref);
export const currentTheme = (): Theme => store.get().theme;
export const setThemePref = (pref: ThemePref): void => writePref(THEME_PREF_KEY, pref);
