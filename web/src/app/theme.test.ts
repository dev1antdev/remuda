import { describe, expect, it } from "vitest";

type Root = { dataset: Record<string, string | undefined>; style: Map<string, string> };

// a document as far as the look touches it: the root's data attributes and custom properties, no theme-color meta
function fakeDocument(): { doc: Document; root: Root } {
  const root: Root = { dataset: {}, style: new Map() };
  const documentElement = {
    dataset: root.dataset,
    style: { setProperty: (name: string, value: string) => root.style.set(name, value), removeProperty: (name: string) => root.style.delete(name) },
  };
  return { doc: { documentElement, querySelector: () => null } as unknown as Document, root };
}

describe("the look on another document", () => {
  it("dresses the pop-out's document at once and follows every change after, until it is let go", async () => {
    const stored = new Map<string, string>();
    const g = globalThis as unknown as { localStorage?: unknown; document?: unknown };
    g.localStorage = { getItem: (k: string) => stored.get(k) ?? null, setItem: (k: string, v: string) => void stored.set(k, v), removeItem: (k: string) => void stored.delete(k) };
    const main = fakeDocument();
    g.document = main.doc;
    try {
      const { startTheme, dressAlso, setThemePref, setDarkFlavorPref, setTextSizePref } = await import("./theme");
      startTheme();
      const popped = fakeDocument();
      const undress = dressAlso(popped.doc);
      expect(popped.root.dataset).toMatchObject({ theme: "dark", flavor: "frappe", accent: "peach" });

      setThemePref("light");
      expect(main.root.dataset.flavor).toBe("latte");
      expect(popped.root.dataset).toMatchObject({ theme: "light", flavor: "latte" });
      setThemePref("dark");
      setDarkFlavorPref("mocha");
      expect(popped.root.dataset.flavor).toBe("mocha");
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
