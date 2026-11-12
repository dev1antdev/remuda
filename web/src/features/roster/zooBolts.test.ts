import { describe, expect, it } from "vitest";
import type { Collision } from "./api";
import { between, boltKey, boltPath, zooBolts } from "./zooBolts";

const collision = (over: Partial<Collision> = {}): Collision => ({
  with: { machine: "local", workspace: "w2", name: "web", dir: "/w/web" },
  files: [{ path: "src/auth.ts", lines: true, from: 40, to: 58 }],
  lines: true,
  first: false,
  why: "pr",
  main: "origin/main",
  ...over,
});

describe("the zoo's lightning", () => {
  const tile = (key: string, machine: string, session: string, collisions?: Collision[]) => ({ key, machine: { id: machine }, session: { id: session, collisions } });
  const peer = (machine: string, workspace: string, lines = true) => collision({ with: { machine, workspace, name: workspace, dir: `/${workspace}` }, lines });

  it("joins two creatures of one pen once, and marks the ones whose peer stands elsewhere", () => {
    const tiles = [
      tile("local|w1|p1", "local", "w1", [peer("local", "w2", false), peer("gog", "w5")]),
      tile("local|w1|p2", "local", "w1", [peer("local", "w2", false), peer("gog", "w5")]), // a second agent of the same workspace
      tile("local|w2|p3", "local", "w2", [peer("local", "w1", false)]),
      tile("gog|w5|p4", "gog", "w5", [peer("local", "w1")]),
      tile("gog|w6|p5", "gog", "w6", [peer("gog", "w7")]), // its peer is not on the wall
      tile("local|w3|p6", "local", "w3"),
    ];
    const { bolts, marked } = zooBolts(tiles);
    expect(bolts).toEqual([{ from: "local|w1|p1", to: "local|w2|p3", lines: false }]);
    expect([...marked]).toEqual([
      ["local|w1|p1", true],
      ["local|w1|p2", true],
      ["gog|w5|p4", true],
      ["gog|w6|p5", true],
    ]);
    expect(boltKey(bolts)).toBe("local|w1|p1>local|w2|p3");
  });

  it("runs from one body's edge to the other's, and not at all between two that touch", () => {
    expect(between({ x: 0, y: 0, r: 10 }, { x: 100, y: 0, r: 20 })).toEqual([10, 0, 80, 0]);
    expect(between({ x: 0, y: 0, r: 30 }, { x: 0, y: 40, r: 30 })).toBeNull();
  });

  it("zigzags across the line between two creatures and ends on both", () => {
    expect(boltPath(0, 0, 60, 0, 2, 5)).toBe("M0 0 L20 5 L40 -5 L60 0");
    expect(boltPath(10, 10, 10, 10).startsWith("M10 10")).toBe(true);
  });
});
