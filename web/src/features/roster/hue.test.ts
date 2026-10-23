import { describe, expect, it } from "vitest";
import { hueStyle, machineHue } from "./hue";

describe("machineHue", () => {
  it("is stable, none for the local machine, and tells the known machines apart", () => {
    expect(machineHue("local")).toBeNull();
    expect(machineHue("")).toBeNull();
    expect(machineHue("gog")).toBe(machineHue("gog"));
    expect(machineHue("gog")).not.toBe(machineHue("roofr"));
    expect(machineHue("gog.remote")).not.toBe(machineHue("roofr.remote"));
  });

  it("never lands on red or green, which mean blocked and done", () => {
    for (const id of ["gog", "roofr", "a", "b", "c", "mac-mini", "build-1", "build-2", "x".repeat(40)]) {
      const hue = machineHue(id)!;
      expect(hue).toBeGreaterThanOrEqual(25);
      expect(hue).toBeLessThanOrEqual(330);
      expect(hue > 95 && hue < 150).toBe(false);
    }
  });

  it("hands the hue over as a CSS variable", () => {
    expect(hueStyle("local")).toBeUndefined();
    expect(hueStyle("gog")).toEqual({ "--machine-hue": machineHue("gog") });
  });
});
