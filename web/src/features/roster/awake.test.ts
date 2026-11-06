import { describe, expect, it } from "vitest";
import { awakeBad, awakeWords } from "./awake";

describe("awakeWords", () => {
  it("says off when the switch is off or unknown", () => {
    expect(awakeWords(undefined)).toBe("Off");
    expect(awakeWords({ on: false, held: false, working: 2 })).toBe("Off");
  });
  it("counts the agents while the hold stands", () => {
    expect(awakeWords({ on: true, held: true, working: 1 })).toBe("Kept awake · 1 agent working");
    expect(awakeWords({ on: true, held: true, working: 3 })).toBe("Kept awake · 3 agents working");
  });
  it("says the hold stands through the grace with nothing working", () => {
    expect(awakeWords({ on: true, held: true, working: 0 })).toBe("Kept awake · nothing working, let go in a couple of minutes");
  });
  it("says why the hold failed", () => {
    expect(awakeWords({ on: true, held: false, working: 1, error: "no sleep inhibitor on box" })).toBe("Could not hold: no sleep inhibitor on box");
  });
  it("says what it does while nothing is held yet", () => {
    expect(awakeWords({ on: true, held: false, working: 2 })).toBe("Taking the hold · 2 agents working");
    expect(awakeWords({ on: true, held: false, working: 0 })).toBe("On · nothing working now");
  });
});

describe("awakeBad", () => {
  it("is the failed hold alone", () => {
    expect(awakeBad({ on: true, held: false, working: 1, error: "x" })).toBe(true);
    expect(awakeBad({ on: false, held: false, working: 1, error: "x" })).toBe(false);
    expect(awakeBad({ on: true, held: true, working: 1 })).toBe(false);
    expect(awakeBad(undefined)).toBe(false);
  });
});
