import { describe, expect, it } from "vitest";
import { moveTarget, movesOf } from "./workflow";

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
