import { describe, expect, it } from "vitest";
import { drawnOn, legibleFor, legibleRules, lift, MIN_CONTRAST, mix, TINTED_ROWS, TOKEN_SLOTS } from "./legible";
import { contrast, FLAVORS, swatches, type Flavor } from "./theme";

const flavors = Object.keys(FLAVORS) as Flavor[];

describe("legible colours on tinted rows", () => {
  it("lifts every token colour to read on every tinted row of every theme", () => {
    for (const flavor of flavors)
      for (const row of TINTED_ROWS)
        for (const slot of [...Object.values(TOKEN_SLOTS), row.ink]) {
          const { colour, ground } = drawnOn(flavor, row, slot);
          const ratio = contrast(colour, ground);
          if (ratio < MIN_CONTRAST - 0.01 && colour !== swatches(flavor).text) throw new Error(`${flavor} ${row.selector} ${slot}: ${colour} on ${ground} reads at ${ratio.toFixed(2)}`);
        }
  });

  it("keeps a colour that reads as it is, and moves one that does not only as far as it must", () => {
    expect(lift("#ffffff", "#000000", "#ffffff")).toBe("#ffffff");
    const dim = lift("#444444", "#303030", "#ffffff");
    expect(contrast(dim, "#303030")).toBeGreaterThanOrEqual(MIN_CONTRAST);
    const steps = Array.from({ length: 21 }, (_, step) => mix("#ffffff", "#444444", step / 20));
    const at = steps.indexOf(dim);
    expect(at).toBeGreaterThan(0);
    expect(contrast(steps[at - 1]!, "#303030")).toBeLessThan(MIN_CONTRAST); // a twentieth less would not have read
  });

  it("mixes a tint over its ground as color-mix does", () => {
    expect(mix("#ff0000", "#000000", 0.5)).toBe("#800000");
    expect(mix("#00ff00", "#ffffff", 0.18)).toBe("#d1ffd1");
  });

  it("writes rules only for the colours it lifted, and keeps each theme's once worked out", () => {
    const rules = legibleRules("latte");
    expect(rules).toMatch(/\.fv-diff \.add \.hl-\w+ \{ color: #[0-9a-f]{6}; \}/);
    expect(rules.split("\n").every((line) => /^[.\w -]+ \{ color: #[0-9a-f]{6};( opacity: 1;)? \}$/.test(line))).toBe(true);
    expect(legibleFor("latte")).toBe(legibleFor("latte"));
  });
});
