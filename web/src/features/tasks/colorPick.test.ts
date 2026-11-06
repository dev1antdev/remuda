import { describe, expect, it } from "vitest";
import css from "../../styles.css?raw";

// rule is the bodies of the stylesheet's rules for one selector alone, as written, one after another
const rule = (selector: string) => [...css.matchAll(new RegExp(`(?:^|\\n)${selector.replace(/\./g, "\\.")} \\{([^}]*)\\}`, "g"))].map((m) => m[1]).join("\n");

// a menu at a fixed point with no offsets sits where the page would have put it before the dialog scrolled: a look at
// 390 × 844 with the project settings scrolled 289 px put the colour menu 265 px under its button
describe("the project settings' colour picker", () => {
  it("opens right under its button, which holds it, as the label and send menus do", () => {
    expect(rule(".color-pick")).toMatch(/position: relative/);
    for (const menu of [".row-menu.color-menu", ".row-menu.label-menu", ".row-menu.split-menu"]) {
      expect(rule(menu), menu).toMatch(/position: absolute;\s*left: 0;\s*top: 100%;/);
    }
  });
});
