import { describe, expect, it } from "vitest";
import { QUOTE_MAX, quoteKeyAsked, quoteText, withQuote } from "./quote";

describe("quoteText", () => {
  it("puts every line behind a quote mark, the blank ones bare", () => {
    expect(quoteText("one\ntwo")).toBe("> one\n> two");
    expect(quoteText("one\n\ntwo")).toBe("> one\n>\n> two");
  });

  it("drops the blank lines at the ends and the trailing spaces, and reads CRLF", () => {
    expect(quoteText("\n\n  one  \r\ntwo   \n\n")).toBe(">   one\n> two");
    expect(quoteText("   \n \n")).toBe("");
    expect(quoteText("")).toBe("");
  });

  it("cuts a very long selection at a line, saying so", () => {
    const line = "x".repeat(100);
    const quoted = quoteText(Array.from({ length: 100 }, () => line).join("\n"));
    expect(quoted.endsWith("> …")).toBe(true);
    expect(quoted.length).toBeLessThan(QUOTE_MAX + 200);
    const huge = quoteText("y".repeat(QUOTE_MAX * 2));
    expect(huge).toBe(`> ${"y".repeat(QUOTE_MAX * 2)}`); // one line is kept whole: nothing to cut it at
  });
});

describe("withQuote", () => {
  it("starts an empty reply with the quote and leaves the caret under it", () => {
    expect(withQuote("", "> a")).toEqual({ draft: "> a\n\n", caret: 5 });
  });

  it("keeps what was typed and puts the quote after it", () => {
    expect(withQuote("first point  \n", "> b")).toEqual({ draft: "first point\n\n> b\n\n", caret: 18 });
  });

  it("changes nothing for an empty quote", () => {
    expect(withQuote("typed", "")).toEqual({ draft: "typed", caret: 5 });
  });
});

describe("quoteKeyAsked", () => {
  const press = (over: Partial<Parameters<typeof quoteKeyAsked>[0]> = {}) => ({ code: "KeyQ", altKey: false, metaKey: false, ctrlKey: false, shiftKey: false, target: { closest: () => null }, ...over });

  it("is Q alone while an answer's text is selected", () => {
    expect(quoteKeyAsked(press(), true)).toBe(true);
    expect(quoteKeyAsked(press(), false)).toBe(false);
    expect(quoteKeyAsked(press({ code: "KeyW" }), true)).toBe(false);
    expect(quoteKeyAsked(press({ metaKey: true }), true)).toBe(false);
    expect(quoteKeyAsked(press({ shiftKey: true }), true)).toBe(false);
  });

  it("leaves a field its letters", () => {
    expect(quoteKeyAsked(press({ target: { closest: () => ({}) } }), true)).toBe(false);
  });
});
