import { describe, expect, it } from "vitest";
import { relocate, sliceRows } from "./selection";

const content = (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, i) => `line ${from + i} of the conversation`);
const memory = (rows: string[], offset = 0) => new Map(rows.map((text, i) => [i + offset, text]));

describe("relocate", () => {
  it("finds the offset most unique rows agree on", () => {
    const seen = memory(content(0, 9));
    expect(relocate([...content(3, 9), "new 10", "new 11", "new 12"], seen, 0)).toBe(3);
    expect(relocate(content(0, 9), seen, 0)).toBe(0);
  });
  it("knows nothing when the screen is somewhere unseen, or too little of it is recognised", () => {
    const seen = memory(content(0, 9));
    expect(relocate(content(20, 29), seen, 0)).toBeNull();
    expect(relocate([...content(8, 9), ...content(30, 37)], seen, 0)).toBeNull();
  });
  it("ignores rows that repeat on screen or in the memory, and short ones", () => {
    const seen = memory(["app header", "a", ...content(0, 7), "app header"]);
    const screen = ["app header", "a", ...content(2, 7), "x", "app header"];
    expect(relocate(screen, seen, 0)).toBe(2);
  });
});

describe("sliceRows", () => {
  it("cuts the selected text out of the remembered rows, trailing spaces trimmed", () => {
    const rows = new Map([
      [0, "hello world   "],
      [1, "second row     "],
      [2, "third"],
    ]);
    expect(sliceRows(rows, { row: 0, col: 6 }, { row: 2, col: 3 })).toBe("world\nsecond row\nthi");
    expect(sliceRows(rows, { row: 1, col: 0 }, { row: 1, col: 6 })).toBe("second");
    expect(sliceRows(rows, { row: 2, col: 0 }, { row: 4, col: 0 })).toBe("third\n\n");
  });
});
