import { describe, expect, it } from "vitest";
import { Terminal } from "@xterm/xterm";
import { WINDOW_OPTIONS, followGrid } from "./grid";

const written = (term: Terminal, data: string) => new Promise<void>((resolve) => term.write(data, resolve));
const row = (term: Terminal, y: number) => term.buffer.active.getLine(y)?.translateToString(true) ?? "";

describe("the grid in a viewer's bytes", () => {
  it("sets xterm's grid before the bytes after it are drawn", async () => {
    const term = new Terminal({ cols: 10, rows: 4, allowProposedApi: true, windowOptions: WINDOW_OPTIONS });
    const told: string[] = [];
    followGrid(term, () => true, (cols, rows) => told.push(`${cols}x${rows}`));
    await written(term, "\x1b[8;6;20t\x1b[H0123456789abcdefghij");
    expect([term.cols, term.rows]).toEqual([20, 6]);
    expect(told).toEqual(["20x6"]);
    expect(row(term, 0)).toBe("0123456789abcdefghij"); // a row of 20, not wrapped at the old 10
  });

  it("is swallowed where nobody follows it, as xterm does without the option", async () => {
    const term = new Terminal({ cols: 10, rows: 4, allowProposedApi: true, windowOptions: WINDOW_OPTIONS });
    followGrid(term, () => false, () => {
      throw new Error("followed");
    });
    await written(term, "a\x1b[8;6;20tb");
    expect([term.cols, term.rows]).toEqual([10, 4]);
    expect(row(term, 0)).toBe("ab");
  });
});
