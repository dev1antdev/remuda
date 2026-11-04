import { describe, expect, it } from "vitest";
import css from "../../styles.css?raw";
import rows from "./rows.tsx?raw";
import tabs from "./TabStrip.tsx?raw";

// SELECT is the Select control's own class: anything else wearing it takes the Select's height, border, padding and type
const SELECT = /\.sel(?![\w-])/;
const SELECT_OWN = new Set([".sel", ".open", ".sm"]);
// the sidebar's rows, the tab strip's tabs and a split tab's panes: this folder's components
const OURS = new Set([".ws-item", ".ws-main", ".ws-agent-line", ".ws-repo-head", ".tab", ".split-pane"]);

// borrowers lists the selectors that put the Select's class on one of our components: a compound such as .tab.sel; a
// pseudo-class's argument (:has(.sel), :not(.quiet)) is another element's or a condition, not the compound's own class
function borrowers(sheet: string): string[] {
  const out: string[] = [];
  const rules = sheet.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/([^{}]+)\{/g);
  for (const [, group] of rules) {
    for (const selector of group!.split(",").map((s) => s.trim())) {
      for (const part of selector.split(/[\s>+~]+/)) {
        const own = part.replace(/\([^()]*\)/g, "");
        if (!SELECT.test(own)) continue;
        const others = (own.match(/\.[\w-]+/g) ?? []).filter((name) => !SELECT_OWN.has(name));
        if (others.some((name) => OURS.has(name))) out.push(selector);
      }
    }
  }
  return out;
}

// wearsSelect says whether a component's source gives an element the class sel
const wearsSelect = (source: string) => /["`\s]sel["`\s]/.test(source);

describe("the selected row, tab and pane", () => {
  it("are marked with a class of their own, never the Select's, which would give them its height, border and padding", () => {
    expect(borrowers(css)).toEqual([]);
    expect(wearsSelect(rows)).toBe(false);
    expect(wearsSelect(tabs)).toBe(false);
  });

  it("still finds a borrower, so the rule above is not empty", () => {
    expect(borrowers(".tab.sel { color: red }\n.sel.open, .radar-settings .sel, .sel:hover, .x:has(.sel) { color: blue }")).toEqual([".tab.sel"]);
    expect(borrowers(".ws-item.on:not(.quiet).sel { color: red }")).toEqual([".ws-item.on:not(.quiet).sel"]);
    expect(wearsSelect('className={`tab${on ? " sel" : ""}`}')).toBe(true);
  });
});
