import { describe, expect, it } from "vitest";
import { SAID_MAX, firstLine } from "./said";

describe("firstLine", () => {
  it("reads a table row as its cells, past the table's rule", () => {
    expect(firstLine("| File | Change |\n|---|:--:|\n| a.go | **fixed** |")).toBe("File · Change");
    expect(firstLine("|:---|---|\n| only | row |")).toBe("only · row");
  });
  it("strips the marks", () => {
    expect(firstLine("\n\n- **Fixed** the `parser` in [x](http://a)\n- more")).toBe("Fixed the parser in x");
    expect(firstLine("> 1. [ ] *note* ~~old~~ <b>new</b>")).toBe("note old new");
    expect(firstLine("```go\nfunc x() {}\n```\nThe code above.")).toBe("The code above.");
    expect(firstLine("---\n\n   \nFirst   real\tline")).toBe("First real line");
    expect(firstLine("snake_case_name stays")).toBe("snake_case_name stays");
    expect(firstLine("")).toBe("");
  });

  it("reads a line of thousands of unclosed links, pictures, tags or marks in its length, not its square", () => {
    const started = performance.now();
    for (const line of ["[".repeat(100_000), "![".repeat(50_000), "[a](".repeat(25_000), "<".repeat(100_000), "- ".repeat(50_000)]) firstLine(line);
    expect(performance.now() - started).toBeLessThan(500); // the square took seconds
    expect(firstLine("see [the docs](https://x.io/a_(b)) and ![logo](l.png) <br> here")).toBe("see the docs) and logo here");
  });

  it("cuts at a word with an ellipsis", () => {
    const got = firstLine("word ".repeat(40));
    expect(got.endsWith("…")).toBe(true);
    expect(got).not.toContain(" …");
    expect(Array.from(got).length).toBeLessThanOrEqual(SAID_MAX + 1);
    expect(Array.from(firstLine("x".repeat(200))).length).toBe(SAID_MAX + 1);
    expect(firstLine("Все тесты прошли, можно мерджить")).toBe("Все тесты прошли, можно мерджить");
  });
});
