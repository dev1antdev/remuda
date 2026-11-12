import { describe, expect, it } from "vitest";
import { parseAnsi } from "../ansi";
import { cellsOf, charWidth, DIM, mix, rgbOf, rowKey } from "./cells";
import { distanceOf, indexer, isInk, Tally } from "./palette";

const defaults = { fg: 0xc6d0f5, bg: 0x303446 };

describe("cellsOf", () => {
  it("lays every character in its column with its colours, the defaults filled in", () => {
    const [line] = parseAnsi("a\x1b[31mb\x1b[0mc", { colors: ["#000000", "#e78284", ...new Array(14).fill("#ffffff")], fg: "#c6d0f5", bg: "#303446" });
    const cells = cellsOf(line!, defaults);
    expect(cells.map((c) => [c.ch, c.fg, c.bg])).toEqual([
      ["a", defaults.fg, defaults.bg],
      ["b", 0xe78284, defaults.bg],
      ["c", defaults.fg, defaults.bg],
    ]);
  });
  it("gives a wide character two cells, a combining mark none, and dims the ink toward the ground", () => {
    const cells = cellsOf([{ text: "日éx", dim: true, bg: "#000000", fg: "#ffffff" }], defaults);
    expect(cells.map((c) => c.ch)).toEqual(["日", "", "é", "x"]);
    expect(cells[0]!.fg).toBe(mix(0xffffff, 0x000000, DIM));
  });
  it("tells a row that moved from one that did not", () => {
    const one = cellsOf([{ text: "ok" }], defaults);
    expect(rowKey(one)).toBe(rowKey(cellsOf([{ text: "ok" }], defaults)));
    expect(rowKey(one)).not.toBe(rowKey(cellsOf([{ text: "ok", bold: true }], defaults)));
    expect(rowKey(one)).not.toBe(rowKey(cellsOf([{ text: "ok", fg: "#ff0000" }], defaults)));
  });
});

describe("colours", () => {
  it("reads CSS hex colours and falls back on anything else", () => {
    expect([rgbOf("#a6d189", 0), rgbOf("#fff", 0), rgbOf("rgb(1,2,3)", 7), rgbOf(undefined, 9)]).toEqual([0xa6d189, 0xffffff, 7, 9]);
  });
  it("counts the measures of a character's width the terminal uses", () => {
    expect([..."a✻⏺│"].map((c) => charWidth(c.codePointAt(0)!))).toEqual([1, 1, 1, 1]);
    expect(["日", "😀", "ｱ"].map((c) => charWidth(c.codePointAt(0)!))).toEqual([2, 2, 1]);
  });
});

describe("the palette", () => {
  it("puts the ground and the ink first, then the most used, at most 256", () => {
    const tally = new Tally();
    tally.add(0x111111, 5);
    tally.add(0x222222, 50);
    tally.add(defaults.bg, 1000);
    for (let i = 0; i < 400; i++) tally.add(0x400000 + i);
    const palette = tally.palette([defaults.bg, defaults.fg]);
    expect(palette.slice(0, 4)).toEqual([defaults.bg, defaults.fg, 0x222222, 0x111111]);
    expect(palette).toHaveLength(256);
  });
  it("finds a colour's place, else the nearest one's", () => {
    const at = indexer([0x000000, 0xffffff, 0xff0000]);
    expect([at(0xffffff), at(0xee1111), at(0x101010)]).toEqual([1, 2, 0]);
    expect(distanceOf(0x000000, 0x000000)).toBe(0);
  });
  it("tells a glyph's soft edge from its ground, so a thin stroke stays whole", () => {
    const edge = (share: number) => {
      const rgb = mix(defaults.fg, defaults.bg, share);
      return isInk((rgb >> 16) & 0xff, (rgb >> 8) & 0xff, rgb & 0xff, defaults.fg, defaults.bg);
    };
    expect([edge(1), edge(0.45), edge(0.25), edge(0)]).toEqual([true, true, false, false]);
    expect(isInk(0x30, 0x34, 0x46, 0x303446, 0x303446)).toBe(false);
  });
});
