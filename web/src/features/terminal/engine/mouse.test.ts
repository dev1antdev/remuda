import { describe, expect, it } from "vitest";
import { modifierBits, report, wheelTicks, type WheelAcc } from "./mouse";

describe("report", () => {
  it("encodes SGR reports with the cell and the press or release", () => {
    expect(report(true, 0, { col: 5, row: 3 }, false)).toBe("\x1b[<0;5;3M");
    expect(report(true, 0, { col: 5, row: 3 }, true)).toBe("\x1b[<0;5;3m");
    expect(report(true, 65, { col: 300, row: 1 }, false)).toBe("\x1b[<65;300;1M");
  });
  it("encodes X10 reports with release as button 3 and coordinates stopping at 223", () => {
    expect(report(false, 0, { col: 1, row: 1 }, false)).toBe("\x1b[M !!");
    expect(report(false, 0, { col: 1, row: 1 }, true)).toBe("\x1b[M#!!");
    expect(report(false, 0, { col: 300, row: 1 }, false)).toBe("\x1b[M " + String.fromCharCode(32 + 223) + "!");
  });
});

describe("wheelTicks", () => {
  it("adds the pixels up, one tick per unit, the rest carried", () => {
    const acc: WheelAcc = { x: 0, y: 0 };
    expect(wheelTicks(acc, 10, false, 20)).toBe(0);
    expect(wheelTicks(acc, 10, false, 20)).toBe(1);
    expect(wheelTicks(acc, 45, false, 20)).toBe(2);
    expect(acc.y).toBe(5);
  });
  it("starts over when the wheel turns the other way, and keeps the axes apart", () => {
    const acc: WheelAcc = { x: 0, y: 15 };
    expect(wheelTicks(acc, -5, false, 20)).toBe(0);
    expect(acc.y).toBe(-5);
    expect(wheelTicks(acc, 40, true, 20)).toBe(2);
    expect(acc).toEqual({ x: 0, y: -5 });
  });
});

describe("modifierBits", () => {
  it("adds shift, alt and ctrl as xterm does", () => {
    expect(modifierBits({ shiftKey: true, altKey: false, ctrlKey: true } as MouseEvent)).toBe(20);
    expect(modifierBits({ shiftKey: false, altKey: true, ctrlKey: false } as MouseEvent)).toBe(8);
  });
});
