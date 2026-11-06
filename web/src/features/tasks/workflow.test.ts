import { describe, expect, it } from "vitest";
import { moveTarget, movesOf, withKind, withMark } from "./workflow";

describe("moveTarget", () => {
  it("keeps the column chosen while it is kept, else the first saved column kept", () => {
    const rows = [{ id: 1 }, { id: 2, removed: true, to: 3 }, { id: 3 }];
    expect(moveTarget(rows[1]!, rows)).toBe(3);
    expect(moveTarget({ id: 2, removed: true }, rows)).toBe(1);
  });
  it("lets go of a column removed in the same edit", () => {
    const rows = [{ id: 1 }, { id: 2, removed: true, to: 3 }, { id: 3, removed: true }, { id: 0 }];
    expect(moveTarget(rows[1]!, rows)).toBe(1);
  });
  it("never names a column the edit adds, which has no id yet", () => {
    const rows = [{ id: 0 }, { id: 2, removed: true }, { id: 5 }];
    expect(moveTarget(rows[1]!, rows)).toBe(5);
  });
});

describe("movesOf", () => {
  it("sends every removed column with a kept column for its tasks", () => {
    const rows = [{ id: 1 }, { id: 2, removed: true, to: 3 }, { id: 3, removed: true, to: 1 }, { id: 4 }];
    expect(movesOf(rows)).toEqual({ 2: 1, 3: 1 });
    expect(movesOf([{ id: 1 }, { id: 2 }])).toEqual({});
  });
});

describe("withMark and withKind", () => {
  const rows = [
    { key: 1, kind: "open", on_new: true, on_start: false, on_finish: false, on_agent: true, on_queue: false },
    { key: 2, kind: "open", on_new: false, on_start: true, on_finish: false, on_agent: false, on_queue: false },
    { key: 3, kind: "done", on_new: false, on_start: false, on_finish: true, on_agent: false, on_queue: false },
  ];
  it("moves a mark to one column, the others lose it", () => {
    expect(withMark(rows, 2, "on_new", true).map((r) => r.on_new)).toEqual([false, true, false]);
  });
  it("never lets the Queued mark share a column with a mark that puts tasks there by themselves", () => {
    const queued = withMark(rows, 1, "on_queue", true)[0]!;
    expect([queued.on_queue, queued.on_new, queued.on_agent]).toEqual([true, false, false]);
    const back = withMark(withMark(rows, 2, "on_queue", true), 2, "on_start", true)[1]!;
    expect([back.on_queue, back.on_start]).toEqual([false, true]);
  });
  it("keeps the Queued mark off a done column", () => {
    const opened = withMark(rows, 3, "on_queue", true)[2]!;
    expect([opened.kind, opened.on_queue, opened.on_finish]).toEqual(["open", true, false]);
    const closed = withKind(withMark(rows, 2, "on_queue", true), 2, "done")[1]!;
    expect([closed.kind, closed.on_queue]).toEqual(["done", false]);
  });
});
