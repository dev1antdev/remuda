import { describe, expect, it } from "vitest";
import { DEFAULT_BG, DEFAULT_FG, parseAnsi } from "./ansi";

describe("parseAnsi", () => {
  it("splits lines on \\n and drops the \\r before it", () => {
    expect(parseAnsi("a\r\nb\nc")).toEqual([[{ text: "a" }], [{ text: "b" }], [{ text: "c" }]]);
    expect(parseAnsi("")).toEqual([[]]);
    expect(parseAnsi("x\n")).toEqual([[{ text: "x" }], []]);
  });

  it("keeps bold, dim, italic, underline and strike until their resets", () => {
    expect(parseAnsi("a\x1b[1mb\x1b[0mc")).toEqual([[{ text: "a" }, { text: "b", bold: true }, { text: "c" }]]);
    expect(parseAnsi("\x1b[2;3;4;9mx\x1b[22;23;24;29my")).toEqual([[{ text: "x", dim: true, italic: true, underline: true, strike: true }, { text: "y" }]]);
    expect(parseAnsi("\x1b[1mx\x1b[my")).toEqual([[{ text: "x", bold: true }, { text: "y" }]]);
  });

  it("draws the basic colours with xterm's palette, brightened by bold", () => {
    expect(parseAnsi("\x1b[31mred\x1b[39m plain")).toEqual([[{ text: "red", fg: "#cc0000" }, { text: " plain" }]]);
    expect(parseAnsi("\x1b[1;31mred")).toEqual([[{ text: "red", fg: "#ef2929", bold: true }]]);
    expect(parseAnsi("\x1b[92mx")).toEqual([[{ text: "x", fg: "#8ae234" }]]);
    expect(parseAnsi("\x1b[44mx\x1b[49my")).toEqual([[{ text: "x", bg: "#3465a4" }, { text: "y" }]]);
    expect(parseAnsi("\x1b[104mx")).toEqual([[{ text: "x", bg: "#729fcf" }]]);
  });

  it("reads the 256-colour table and true colour, in either spelling", () => {
    expect(parseAnsi("\x1b[38;5;196mx")).toEqual([[{ text: "x", fg: "#ff0000" }]]);
    expect(parseAnsi("\x1b[48;5;240mx")).toEqual([[{ text: "x", bg: "#585858" }]]);
    expect(parseAnsi("\x1b[38;5;9mx")).toEqual([[{ text: "x", fg: "#ef2929" }]]);
    expect(parseAnsi("\x1b[38;2;177;185;249mx")).toEqual([[{ text: "x", fg: "#b1b9f9" }]]);
    expect(parseAnsi("\x1b[38:2::177:185:249mx")).toEqual([[{ text: "x", fg: "#b1b9f9" }]]);
    expect(parseAnsi("\x1b[38:5:196mx")).toEqual([[{ text: "x", fg: "#ff0000" }]]);
    expect(parseAnsi("\x1b[38;2;1;2;3;1mx")).toEqual([[{ text: "x", fg: "#010203", bold: true }]]);
  });

  it("swaps the colours for inverse, with the defaults where a side has none", () => {
    expect(parseAnsi("\x1b[7minv\x1b[27mn")).toEqual([[{ text: "inv", fg: DEFAULT_BG, bg: DEFAULT_FG }, { text: "n" }]]);
    expect(parseAnsi("\x1b[31;7mx")).toEqual([[{ text: "x", fg: DEFAULT_BG, bg: "#cc0000" }]]);
  });

  it("drops every other escape and control byte", () => {
    expect(parseAnsi("\x1b]0;a title\x07x")).toEqual([[{ text: "x" }]]);
    expect(parseAnsi("\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\")).toEqual([[{ text: "link" }]]);
    expect(parseAnsi("\x1b[2J\x1b[H\x1b[?25lx\x1b(B\x1b7y\x07")).toEqual([[{ text: "xy" }]]);
    expect(parseAnsi("\x1b[1;31")).toEqual([[]]);
    expect(parseAnsi("\x1b")).toEqual([[]]);
  });

  it("keeps any alphabet, tabs and emoji", () => {
    expect(parseAnsi("привет\t😀 \x1b[1mжирный")).toEqual([[{ text: "привет\t😀 " }, { text: "жирный", bold: true }]]);
  });

  it("reads a screen as herdr prints it", () => {
    const screen = "    \x1b[0m\x1b[38;2;177;185;249mEdit chat.go\x1b[0m, плюс\r\n  - \x1b[0m\x1b[1mЛента\x1b[0m текст\r\n";
    expect(parseAnsi(screen)).toEqual([
      [{ text: "    " }, { text: "Edit chat.go", fg: "#b1b9f9" }, { text: ", плюс" }],
      [{ text: "  - " }, { text: "Лента", bold: true }, { text: " текст" }],
      [],
    ]);
  });
});
