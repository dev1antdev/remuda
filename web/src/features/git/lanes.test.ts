import { describe, expect, it } from "vitest";
import { foldLanes, startLanes, type LaneCommit, type LaneRow } from "./lanes";

const seg = (from: number, fromY: number, to: number, toY: number, color: number, dashed = false) => ({ from, fromY, to, toY, color, dashed });

// a history: main with a merge of feature, a fix branch open, the uncommitted changes on top
const HISTORY: LaneCommit[] = [
  { hash: "*", parents: ["m"], dashed: true },
  { hash: "x1", parents: ["three"] },
  { hash: "m", parents: ["three", "f2"] },
  { hash: "f2", parents: ["f1"] },
  { hash: "three", parents: ["two"] },
  { hash: "f1", parents: ["two"] },
  { hash: "two", parents: ["init"] },
  { hash: "init", parents: [] },
];

// every line entering a row goes on from one leaving the row above
function continuous(rows: LaneRow[]) {
  for (let i = 1; i < rows.length; i++) {
    const below = [...new Set(rows[i - 1]!.segments.filter((s) => s.toY === 1).map((s) => s.to))].sort();
    const above = [...new Set(rows[i]!.segments.filter((s) => s.fromY === 0).map((s) => s.from))].sort();
    expect(above, `row ${i}`).toEqual(below);
  }
}

describe("foldLanes", () => {
  it("keeps a lane straight until its commit comes, bending only at forks and merges", () => {
    const { rows, state } = foldLanes(HISTORY);
    continuous(rows);
    expect(rows[0]!.segments).toEqual([seg(0, 0.5, 0, 1, 0, true)]);
    const merge = rows[2]!;
    expect(merge.segments.some((s) => s.dashed && s.toY === 0.5)).toBe(true); // the dashed line ends at HEAD
    expect(merge.segments.filter((s) => s.fromY === 0.5).every((s) => !s.dashed)).toBe(true);
    expect(merge.segments.filter((s) => s.fromY === 0.5)).toHaveLength(2);
    expect(state.width).toBeGreaterThanOrEqual(3);
    expect(state.lanes).toEqual([]);
  });

  it("goes on from where the last page ended, as one fold of the whole would", () => {
    const whole = foldLanes(HISTORY);
    const first = foldLanes(HISTORY.slice(0, 4));
    const second = foldLanes(HISTORY.slice(4), first.state);
    expect([...first.rows, ...second.rows]).toEqual(whole.rows);
    expect(second.state).toEqual(whole.state);
    expect(first.state.lanes.length).toBeGreaterThan(0); // a page that ends mid-branch carries its lanes
  });

  it("never changes the state it was handed", () => {
    const first = foldLanes(HISTORY.slice(0, 3));
    const kept = JSON.stringify(first.state);
    foldLanes(HISTORY.slice(3), first.state);
    expect(JSON.stringify(first.state)).toBe(kept);
  });

  it("draws an octopus merge, a parent past the pages read and nothing at all", () => {
    expect(foldLanes([])).toEqual({ rows: [], state: startLanes() });
    const octopus = foldLanes([{ hash: "m", parents: ["a", "b", "c"] }, { hash: "a", parents: [] }, { hash: "b", parents: [] }, { hash: "c", parents: [] }]);
    expect(octopus.rows[0]!.segments).toEqual([seg(0, 0.5, 0, 1, 0), seg(0, 0.5, 1, 1, 1), seg(0, 0.5, 2, 1, 2)]);
    expect(foldLanes([{ hash: "x", parents: ["gone"] }]).rows[0]!.segments).toEqual([seg(0, 0.5, 0, 1, 0)]);
  });
});
