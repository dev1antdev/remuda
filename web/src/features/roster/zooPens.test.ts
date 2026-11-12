import { describe, expect, it } from "vitest";
import type { Health } from "./api";
import { CLEAR, pensOf, skyOf, weatherOf, weatherWords } from "./zooPens";

const health = (over: Partial<Health> = {}): Health => ({ at: "", disk_free_gb: 40, disk_used_pct: 50, load1: 0.4, cores: 4, ...over });

describe("pensOf", () => {
  it("groups the creatures by machine in the order they first stand", () => {
    const tile = (machine: string, pane: string) => ({ machine: { id: machine }, pane });
    const pens = pensOf([tile("gog", "a"), tile("mac", "b"), tile("gog", "c")]);
    expect(pens.map((p) => [p.machineId, p.tiles.map((t) => t.pane)])).toEqual([
      ["gog", ["a", "c"]],
      ["mac", ["b"]],
    ]);
    expect(pensOf([])).toEqual([]);
  });
});

describe("weatherOf", () => {
  it("is clear for a machine in good shape", () => {
    expect(weatherOf(health(), undefined)).toEqual(CLEAR);
    expect(weatherOf(undefined, undefined)).toEqual(CLEAR);
  });

  it("turns warm from 75 °C and hot from 85 °C", () => {
    expect(weatherOf(health({ temp_c: 76 }), undefined).heat).toBe("warm");
    expect(weatherOf(health({ temp_c: 86 }), undefined).heat).toBe("hot");
  });

  it("storms on under-voltage, piles boxes on a nearly full disk, raises dust past the cores", () => {
    const weather = weatherOf(health({ throttle: { now: ["under-voltage"] }, disk_used_pct: 93, load1: 6.2 }), undefined);
    expect(weather).toMatchObject({ storm: true, boxes: true, dust: true });
  });

  it("lights the lamp only while the keep-awake hold stands", () => {
    expect(weatherOf(health(), { on: true, held: true, working: 1 }).lamp).toBe(true);
    expect(weatherOf(health(), { on: true, held: false, working: 0 }).lamp).toBe(false);
    expect(weatherOf(undefined, { on: true, held: true, working: 2 }).lamp).toBe(true);
  });

  it("says the weather on the pen's sign", () => {
    const h = health({ temp_c: 62.4, load1: 0.42 });
    expect(weatherWords(h, weatherOf(h, undefined))).toBe("62 °C · load 0.4");
    const stormy = health({ throttle: { now: ["under-voltage"] }, disk_used_pct: 95, load1: 1 });
    expect(weatherWords(stormy, weatherOf(stormy, { on: true, held: true, working: 1 }))).toBe("under-voltage · load 1.0 · disk 95% full · kept awake");
  });
});

describe("skyOf", () => {
  it("follows the viewer's hour", () => {
    expect([4, 5, 8, 12, 18, 20, 21, 23].map(skyOf)).toEqual(["night", "dawn", "day", "day", "dusk", "dusk", "night", "night"]);
  });
});
