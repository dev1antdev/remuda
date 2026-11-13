import { describe, expect, it } from "vitest";
import editorThemes from "../../../../internal/code/themes.go?raw";
import { DARK_FLAVORS, LIGHT_FLAVORS } from "@/lib/theme";

describe("the editor's themes", () => {
  it("name exactly the page's twenty, so the Code mode wears whichever is picked", () => {
    const table = editorThemes.slice(editorThemes.indexOf("var editorThemes"));
    const named = [...table.matchAll(/^\t"([a-z-]+)":\s+\{/gm)].map((m) => m[1]).sort();
    expect(named).toEqual([...DARK_FLAVORS, ...LIGHT_FLAVORS].sort());
  });
});
