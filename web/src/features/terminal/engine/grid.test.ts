import { describe, expect, it } from "vitest";
import { Terminal } from "@xterm/xterm";
import { atGrid, validGrid } from "./grid";
import { gridHeader, gridOf } from "./transport";

const written = (term: Terminal, data: string) => new Promise<void>((resolve) => term.write(data, resolve));
const row = (term: Terminal, y: number) => term.buffer.active.getLine(y)?.translateToString(true) ?? "";

describe("the grid a viewer is told", () => {
  it("takes effect after the bytes before it and before the bytes after it", async () => {
    const term = new Terminal({ cols: 10, rows: 4, allowProposedApi: true });
    const told: string[] = [];
    term.write("0123456789ab"); // drawn at 10 columns: wraps
    atGrid(term, 20, 6, (cols, rows) => told.push(`${cols}x${rows}`));
    await written(term, "\r\n0123456789abcdefghij"); // drawn at 20: one row
    expect([term.cols, term.rows]).toEqual([20, 6]);
    expect(told).toEqual(["20x6"]);
    expect(row(term, 0)).toBe("0123456789");
    expect(row(term, 2)).toBe("0123456789abcdefghij");
  });

  it("is never the program's own: a window-size request in the bytes leaves the grid alone", async () => {
    const term = new Terminal({ cols: 10, rows: 4, allowProposedApi: true });
    await written(term, "a\x1b[8;6;20tb");
    expect([term.cols, term.rows]).toEqual([10, 4]);
    expect(row(term, 0)).toBe("ab");
  });

  it("refuses a grid no terminal holds", () => {
    expect(validGrid(100, 30)).toBe(true);
    expect(validGrid(0, 30)).toBe(false);
    expect(validGrid(100000, 30)).toBe(false);
    expect(validGrid(100, 1e6)).toBe(false);
    expect(validGrid(1.5, 30)).toBe(false);
    const term = new Terminal({ cols: 10, rows: 4, allowProposedApi: true });
    atGrid(term, 100000, 100000, () => {
      throw new Error("applied");
    });
  });

  it("reads the grid off the socket's text frame and the HTTP header", () => {
    expect(gridOf('{"cols":100,"rows":30,"type":"grid"}')).toEqual({ cols: 100, rows: 30 });
    expect(gridOf('{"type":"other"}')).toBeNull();
    expect(gridOf("not json")).toBeNull();
    expect(gridHeader("90x28")).toEqual({ cols: 90, rows: 28 });
    expect(gridHeader(null)).toBeNull();
    expect(gridHeader("90x")).toBeNull();
  });
});
