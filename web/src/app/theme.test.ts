import { describe, expect, it } from "vitest";

type Root = { dataset: Record<string, string | undefined>; style: Map<string, string>; sheets: { id: string; textContent: string }[] };

// a document as far as the look touches it: the root's data attributes and custom properties, the head's style
// elements, no theme-color meta
function fakeDocument(): { doc: Document; root: Root } {
  const root: Root = { dataset: {}, style: new Map(), sheets: [] };
  const documentElement = {
    dataset: root.dataset,
    style: { setProperty: (name: string, value: string) => root.style.set(name, value), removeProperty: (name: string) => root.style.delete(name) },
  };
  const head = { appendChild: (sheet: { id: string; textContent: string }) => root.sheets.push(sheet) };
  const doc = { documentElement, head, querySelector: () => null, getElementById: (id: string) => root.sheets.find((sheet) => sheet.id === id) ?? null, createElement: () => ({ id: "", textContent: "" }) };
  return { doc: doc as unknown as Document, root };
}

describe("the look on another document", () => {
  it("dresses the pop-out's document at once and follows every change after, until it is let go", async () => {
    const stored = new Map<string, string>();
    const g = globalThis as unknown as { localStorage?: unknown; document?: unknown };
    g.localStorage = { getItem: (k: string) => stored.get(k) ?? null, setItem: (k: string, v: string) => void stored.set(k, v), removeItem: (k: string) => void stored.delete(k) };
    const main = fakeDocument();
    g.document = main.doc;
    try {
      stored.set("remuda.theme.light", JSON.stringify("sepia"));
      const { startTheme, dressAlso, setThemePref, setDarkFlavorPref, setLightFlavorPref, setTextSizePref } = await import("./theme");
      startTheme();
      const popped = fakeDocument();
      const undress = dressAlso(popped.doc);
      expect(popped.root.dataset).toMatchObject({ theme: "dark", flavor: "frappe", accent: "peach" });

      setThemePref("light");
      expect(main.root.dataset.flavor).toBe("latte");
      expect(popped.root.dataset).toMatchObject({ theme: "light", flavor: "latte" });
      setLightFlavorPref("solarized-light");
      expect(popped.root.dataset).toMatchObject({ theme: "light", flavor: "solarized-light" });
      setThemePref("dark");
      setDarkFlavorPref("nord");
      expect(popped.root.dataset).toMatchObject({ theme: "dark", flavor: "nord" });
      setDarkFlavorPref("mocha");
      expect(popped.root.dataset.flavor).toBe("mocha");
      const { legibleFor } = await import("@/lib/legible");
      expect(popped.root.sheets.map((sheet) => sheet.textContent)).toEqual([legibleFor("mocha")]); // one sheet, the theme's own lifted rows
      setTextSizePref("large");
      expect(popped.root.dataset.text).toBe("large");

      undress();
      setDarkFlavorPref("macchiato");
      expect(main.root.dataset.flavor).toBe("macchiato");
      expect(popped.root.dataset.flavor).toBe("mocha");
    } finally {
      delete g.localStorage;
      delete g.document;
    }
  });
});
