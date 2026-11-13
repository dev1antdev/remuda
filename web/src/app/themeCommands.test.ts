import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { themeCommands, type ThemeSetters } from "./themeCommands";

const picked: string[] = [];
const set: ThemeSetters = { mode: (p) => picked.push(`mode ${p}`), dark: (f) => picked.push(`dark ${f}`), light: (f) => picked.push(`light ${f}`) };

describe("the palette's theme rows", () => {
  afterEach(() => setLang("en"));

  it("offer the mode, then each of the ten dark and ten light themes, the ones in use marked now", () => {
    const rows = themeCommands({ mode: "system", dark: "nord", light: "solarized-light" }, set);
    expect(rows.map((r) => r.id).slice(0, 3)).toEqual(["theme-system", "theme-dark", "theme-light"]);
    expect(rows.filter((r) => r.id.startsWith("theme-dark-"))).toHaveLength(10);
    expect(rows.filter((r) => r.id.startsWith("theme-light-"))).toHaveLength(10);
    expect(rows[0]!.meta.hint).toBe("now · Nord when the system is dark, Solarized Light when it is light");
    expect(rows[1]!.meta.hint).toBe("Nord, whatever the system says");
    const nord = rows.find((r) => r.id === "theme-dark-nord")!;
    expect(nord.title).toBe("Dark theme: Nord");
    expect(nord.meta.hint).toBe("now · whenever the page is dark");
    expect(rows.find((r) => r.id === "theme-dark-dracula")!.meta.hint).toBe("whenever the page is dark");
    expect(rows.find((r) => r.id === "theme-light-rose-pine-dawn")!.meta.keywords).toEqual(expect.arrayContaining(["rosé", "rose", "pine", "dawn", "light"]));
  });

  it("set what they name", () => {
    picked.length = 0;
    const rows = themeCommands({ mode: "dark", dark: "frappe", light: "latte" }, set);
    for (const id of ["theme-light", "theme-dark-kanagawa", "theme-light-github-light"]) rows.find((r) => r.id === id)!.run();
    expect(picked).toEqual(["mode light", "dark kanagawa", "light github-light"]);
  });

  it("speak the page's language", () => {
    setLang("ru");
    const rows = themeCommands({ mode: "light", dark: "frappe", light: "latte" }, set);
    expect(rows.find((r) => r.id === "theme-dark-one-dark")!.title).toBe("Тёмная тема: One Dark");
    expect(rows.find((r) => r.id === "theme-light")!.title).toBe("Тема: всегда светлая");
  });
});
