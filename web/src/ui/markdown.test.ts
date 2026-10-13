import { describe, expect, it } from "vitest";
import { parseBlocks } from "./Markdown";

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
});
