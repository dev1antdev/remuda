import { describe, expect, it } from "vitest";
import { cellText } from "./cellText";

describe("cellText", () => {
  it("gives the words of a cell without its marks", () => {
    expect(cellText("**bold** and _soft_ and `code` and ~~gone~~")).toBe("bold and soft and code and gone");
    expect(cellText("see [the docs](https://example.com/x) now")).toBe("see the docs now");
    expect(cellText("plain https://example.com/a,")).toBe("plain https://example.com/a,");
  });
  it("keeps a line break written as <br> and trims the ends", () => {
    expect(cellText("first line<br>second line")).toBe("first line\nsecond line");
    expect(cellText("  one <br/> two  ")).toBe("one \n two");
  });
  it("leaves nested marks readable", () => {
    expect(cellText("**[bold link](https://a.b)** _it `c`_")).toBe("bold link it c");
  });
  it("opens a code span on a whole run of backticks, in linear time", () => {
    expect(cellText("``a`b`` and `` ` ``")).toBe("a`b and  `");
    expect(cellText("``a` stays")).toBe("``a` stays");
    const started = performance.now();
    expect(cellText("`".repeat(9_999))).toBe("`".repeat(9_999));
    expect(performance.now() - started).toBeLessThan(200);
  });
});
