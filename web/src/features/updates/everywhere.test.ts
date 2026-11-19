import { describe, expect, it } from "vitest";
import type { StepRun } from "./api";
import { byMachine, hostRestarting, stateWords, summaryWords } from "./everywhere";

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
  it("reads a run gone with the page's own remuda as its restart, not as a failure", () => {
    expect(hostRestarting([step("gog", "herdr", "ok"), step("local", "remuda-release", "running")], "local")).toBe(true);
    expect(hostRestarting([step("local", "remuda-release", "ok")], "local")).toBe(true);
    expect(hostRestarting([step("gog", "herdr", "running"), step("local", "remuda-release")], "local")).toBe(false);
    expect(hostRestarting([step("gog", "remuda-release", "running")], "local")).toBe(false);
    expect(hostRestarting([step("local", "remuda-release", "failed")], "local")).toBe(false);
  });
});
