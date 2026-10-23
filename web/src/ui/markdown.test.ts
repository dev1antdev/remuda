import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Markdown, parseBlocks } from "./Markdown";

describe("parseBlocks", () => {
  it("reads headings, paragraphs, task lists and fenced code", () => {
    const blocks = parseBlocks("# Title\n\npara line one\npara line two\n\n- [ ] todo\n- [x] done\n- plain\n\n```go\nfmt.Println()\n```\n");
    expect(blocks).toMatchObject([
      { t: "h", level: 1, text: "Title" },
      { t: "p", lines: ["para line one", "para line two"] },
      { t: "list", ordered: false, items: [{ checked: false }, { checked: true }, { checked: null }] },
      { t: "code", lang: "go", code: "fmt.Println()" },
    ]);
  });
  it("reads ordered lists from their first number, quotes, rules and tables", () => {
    const blocks = parseBlocks("3. three\n4. four\n\n> quoted\n\n---\n\n| a | b |\n|---|---|\n| 1 | 2 |\n");
    expect(blocks).toMatchObject([{ t: "list", ordered: true, start: 3 }, { t: "quote" }, { t: "hr" }, { t: "table", head: ["a", "b"], rows: [["1", "2"]] }]);
  });
  it("keeps CRLF text whole", () => {
    expect(parseBlocks("one\r\ntwo\r\n")).toMatchObject([{ t: "p", lines: ["one", "two"] }]);
  });
  it("stops nesting quotes and lists at a depth, so a line of thousands of marks is text, not a stack overflow", () => {
    const quotes = parseBlocks(">".repeat(5000) + "x");
    let block = quotes[0]!;
    let depth = 0;
    while (block.t === "quote") {
      depth++;
      block = block.blocks[0]!;
    }
    expect(depth).toBe(16);
    expect(block).toMatchObject({ t: "p", lines: [">".repeat(5000 - 16) + "x"] });
    expect(() => parseBlocks("- ".repeat(5000) + "x")).not.toThrow();
    expect(() => parseBlocks("1. ".repeat(5000) + "x")).not.toThrow();
  });
});

describe("Markdown", () => {
  const draw = (text: string) => renderToStaticMarkup(createElement(Markdown, { text, onOpenFile: () => {} }));
  it("marks and links a line of prose", () => {
    const html = draw("see **main.go** and `x`");
    expect(html).toContain("<strong>");
    expect(html).toContain('class="path-link"');
  });
  it("draws a line over ten thousand characters as plain text, in milliseconds", () => {
    const started = performance.now();
    const html = draw("[".repeat(40_000) + " **not bold** main.go");
    expect(html).not.toContain("<strong>");
    expect(html).not.toContain("path-link");
    expect(performance.now() - started).toBeLessThan(500);
  });
});
