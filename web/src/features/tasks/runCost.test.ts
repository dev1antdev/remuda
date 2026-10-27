import { describe, expect, it } from "vitest";
import { runSeconds, runWords, taskCost } from "./runCost";

const now = Date.parse("2026-10-27T12:40:00Z");
const started = "2026-10-27T12:00:00Z";

describe("runSeconds", () => {
  it("is the run's own figure once it finished", () => {
    expect(runSeconds({ state: "finished", started, took: 2280 }, now)).toBe(2280);
  });
  it("counts from the start while the agent still works", () => {
    expect(runSeconds({ state: "running", started }, now)).toBe(2400);
    expect(runSeconds({ state: "running", started, took: 0 }, now)).toBe(2400);
  });
  it("is nothing for a pane gone before any finish", () => {
    expect(runSeconds({ state: "gone", started }, now)).toBe(0);
    expect(runSeconds({ state: "failed", started }, now)).toBe(0);
    expect(runSeconds({ state: "gone", started, took: 90 }, now)).toBe(90);
  });
});

describe("runWords", () => {
  it("says the cost, the time and the model", () => {
    expect(runWords({ state: "finished", started, cost: 4.2, took: 2280, model: "Opus 4.5" }, now)).toBe("$4.20 · 38 min · Opus 4.5");
  });
  it("leaves out what is not known", () => {
    expect(runWords({ state: "running", started }, now)).toBe("40 min");
    expect(runWords({ state: "running", started, cost: 0.5 }, now)).toBe("$0.50 · 40 min");
    expect(runWords({ state: "gone", started }, now)).toBe("");
    expect(runWords({ state: "finished", started, took: 4000, model: "Sonnet 4.5" }, now)).toBe("1 h 7 min · Sonnet 4.5");
  });
});

describe("taskCost", () => {
  it("is the server's sum, else the latest run's figure, else nothing", () => {
    expect(taskCost({ cost: 3.5, run: { cost: 1 } as never })).toBe(3.5);
    expect(taskCost({ run: { cost: 1 } as never })).toBe(1);
    expect(taskCost({})).toBe(0);
  });
});
