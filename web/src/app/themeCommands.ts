// The palette's rows for the look's colours: the mode (follow the system, always dark, always light), then a row per
// dark theme and per light theme. Pure, so commands.tsx only lists them.

import type { Entry } from "@/features/palette/rank";
import { t } from "@/lib/i18n";
import { DARK_FLAVORS, FLAVOR_NAMES, LIGHT_FLAVORS, type DarkFlavor, type LightFlavor, type ThemePref } from "@/lib/theme";

export type ThemeRow = { id: string; title: string; run: () => void; meta: Partial<Entry> };
export type ThemeNow = { mode: ThemePref; dark: DarkFlavor; light: LightFlavor };
export type ThemeSetters = { mode: (pref: ThemePref) => void; dark: (flavor: DarkFlavor) => void; light: (flavor: LightFlavor) => void };

const now = (on: boolean) => (on ? t("now · ") : "");

// nameWords are a theme's name as words to type, accents and all, and without them
const nameWords = (name: string): string[] => {
  const words = name.toLowerCase().split(/\s+/);
  const plain = words.map((w) => w.normalize("NFD").replace(/[̀-ͯ]/g, ""));
  return [...new Set([...words, ...plain])];
};

// themeCommands are the rows, the one in use marked "now"
export function themeCommands(current: ThemeNow, set: ThemeSetters): ThemeRow[] {
  const dark = FLAVOR_NAMES[current.dark];
  const light = FLAVOR_NAMES[current.light];
  const rows: ThemeRow[] = [
    { id: "theme-system", title: t("Theme: follow the system"), run: () => set.mode("system"), meta: { hint: now(current.mode === "system") + t("{dark} when the system is dark, {light} when it is light", { dark, light }), icon: "monitor", keywords: ["theme", "dark", "light", "system", "auto", "colours", "colors"] } },
    { id: "theme-dark", title: t("Theme: always dark"), run: () => set.mode("dark"), meta: { hint: now(current.mode === "dark") + t("{theme}, whatever the system says", { theme: dark }), icon: "moon", keywords: ["theme", "dark", "night", ...nameWords(dark)] } },
    { id: "theme-light", title: t("Theme: always light"), run: () => set.mode("light"), meta: { hint: now(current.mode === "light") + t("{theme}, whatever the system says", { theme: light }), icon: "sun", keywords: ["theme", "light", "day", ...nameWords(light)] } },
  ];
  for (const flavor of DARK_FLAVORS)
    rows.push({ id: `theme-dark-${flavor}`, title: t("Dark theme: {theme}", { theme: FLAVOR_NAMES[flavor] }), run: () => set.dark(flavor), meta: { hint: now(current.dark === flavor) + t("whenever the page is dark"), icon: "moon", keywords: ["theme", "dark", "colours", "colors", ...nameWords(FLAVOR_NAMES[flavor])] } });
  for (const flavor of LIGHT_FLAVORS)
    rows.push({ id: `theme-light-${flavor}`, title: t("Light theme: {theme}", { theme: FLAVOR_NAMES[flavor] }), run: () => set.light(flavor), meta: { hint: now(current.light === flavor) + t("whenever the page is light"), icon: "sun", keywords: ["theme", "light", "colours", "colors", ...nameWords(FLAVOR_NAMES[flavor])] } });
  return rows;
}
