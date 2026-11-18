import { describe, expect, it } from "vitest";
import doc from "../../../docs/design.md?raw";
import css from "../styles.css?raw";

// spans are the doc's code spans
const spans = [...doc.matchAll(/`([^`\n]+)`/g)].map((m) => m[1]!);

// properties are the custom properties the doc names, a placeholder (--ctp-<slot>) left out
const properties = [...new Set(spans.flatMap((span) => span.match(/--[a-z0-9-]+(?![<\w-])/g) ?? []))];

// classes are the class names of the spans that are selectors (`.btn.primary`, `.keybar .key`), placeholders left out
const classes = [...new Set(spans.filter((span) => span.startsWith(".")).flatMap((span) => span.split(/[\s.]+/).filter((name) => name && !/[<>]/.test(name))))];

describe("docs/design.md, the design book", () => {
  it("names custom properties and classes at all, so the checks below read something", () => {
    expect(properties.length).toBeGreaterThan(40);
    expect(classes.length).toBeGreaterThan(20);
  });

  it("names only custom properties the stylesheet declares", () => {
    const missing = properties.filter((name) => !css.includes(`${name}:`));
    expect(missing).toEqual([]);
  });

  it("names only classes the stylesheet styles", () => {
    const missing = classes.filter((name) => !new RegExp(`\\.${name.replace(/[-]/g, "\\-")}(?![\\w-])`).test(css));
    expect(missing).toEqual([]);
  });
});
