import { describe, expect, it } from "vitest";
import css from "../../styles.css?raw";

const sheet = css.replace(/\/\*[\s\S]*?\*\//g, "");

// rules are every innermost rule of the stylesheet, a media block's too, as its selectors and its body
const rules = [...sheet.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map(([, group, body]) => ({ selectors: group!.split(",").map((s) => s.trim()), body: body! }));

// blockAt is the text of the block whose opening brace ends just before start, to its matching closing brace
function blockAt(start: number): string {
  let depth = 1;
  let end = start;
  for (; end < sheet.length && depth > 0; end++) {
    if (sheet[end] === "{") depth++;
    else if (sheet[end] === "}") depth--;
  }
  return sheet.slice(start, end);
}

// stilled is every selector a reduced-motion block stops the animations of
function stilled(): Set<string> {
  const out = new Set<string>();
  for (const at of sheet.matchAll(/@media \(prefers-reduced-motion: reduce\)\s*\{/g)) {
    for (const [, group, body] of blockAt(at.index! + at[0].length).matchAll(/([^{}]+)\{([^{}]*)\}/g))
      if (/animation:\s*none/.test(body!)) for (const selector of group!.split(",")) out.add(selector.trim());
  }
  return out;
}

describe("the zoo under reduced motion", () => {
  it("stills the pseudo-elements that move too, which .zoo * does not reach: a storm's flash is one", () => {
    const still = stilled();
    const moving = rules
      .filter((rule) => /animation(-name)?:\s*(?!none)/.test(rule.body))
      .flatMap((rule) => rule.selectors)
      .filter((selector) => selector.startsWith(".zoo") && /::(before|after)$/.test(selector));
    expect(moving).toContain(".zoo-pen.is-storm::before");
    const loose = moving.filter((selector) => !still.has(selector) && !still.has(`.zoo *::${selector.split("::")[1]}`));
    expect(loose).toEqual([]);
  });
});
