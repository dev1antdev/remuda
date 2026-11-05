import { describe, expect, it } from "vitest";
import type { StepRun } from "./api";
import { byMachine, restartsHost, stateWords, summaryWords } from "./everywhere";

const step = (machine: string, what: string, state: StepRun["state"] = "waiting", error?: string): StepRun => ({ machine, label: machine.toUpperCase(), what, title: what, command: what, state, error });

describe("Update everywhere", () => {
  it("groups the steps by machine in the server's order", () => {
    const groups = byMachine([step("gog", "herdr"), step("gog", "claude"), step("roofr", "claude"), step("local", "remuda-release")]);
    expect(groups.map((g) => [g.label, g.steps.map((s) => s.what)])).toEqual([
      ["GOG", ["herdr", "claude"]],
      ["ROOFR", ["claude"]],
      ["LOCAL", ["remuda-release"]],
    ]);
  });
  it("says how far a run is, then what it came to", () => {
    const steps = [step("gog", "herdr", "ok"), step("gog", "claude", "failed", "npm refused"), step("roofr", "claude", "running"), step("x", "kimi", "skipped")];
    expect(summaryWords(steps, false)).toBe("3 of 4 done");
    expect(summaryWords(steps.map((s) => (s.state === "running" ? { ...s, state: "ok" as const } : s)), true)).toBe("2 updated, 1 failed, 1 skipped");
    expect(stateWords(steps[1]!)).toBe("failed: npm refused");
    expect(stateWords(steps[2]!)).toBe("running…");
  });
  it("knows when the run ends with the page's own remuda restarting", () => {
    expect(restartsHost([step("gog", "remuda-release"), step("local", "remuda-release")], "local")).toBe(true);
    expect(restartsHost([step("gog", "remuda-release")], "local")).toBe(false);
    expect(restartsHost([step("local", "remuda-release")], undefined)).toBe(false);
  });
});
