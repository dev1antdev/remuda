import { describe, expect, it } from "vitest";
import type { FleetMachine } from "./api";
import { factsOf, nextWords, noRoomWords, outWords } from "./fleet";

const machine = (over: Partial<FleetMachine> = {}): FleetMachine => ({ machine: "gog", label: "GOG", dir: "/srv/app", load_per_core: 0.2, plan: -1, working: 0, ...over });

describe("a fleet queue's words", () => {
  it("says a machine's figures, warm, on battery and the plan only when they say something", () => {
    expect(factsOf(machine(), "claude")).toBe("load 0.2 per core, no agent working");
    expect(factsOf(machine({ temp_c: 78, battery: { percent: 60, discharging: true }, working: 3, plan: 41.6 }), "codex")).toBe("load 0.2 per core, warm at 78 °C, on battery at 60 %, 3 agents working, Codex at 42 %");
    expect(factsOf(machine({ battery: { percent: 60 } }), "claude")).not.toContain("battery");
  });

  it("says why each machine has no room, the plan and the budget in the server's words", () => {
    expect(outWords(machine({ out: "hot", temp_c: 88.4 }))).toBe("GOG: hot at 88 °C");
    expect(outWords(machine({ out: "battery", battery: { percent: 12, discharging: true } }))).toBe("GOG: on battery at 12 %");
    expect(outWords(machine({ out: "load", load_per_core: 2.44 }))).toBe("GOG: load 2.4 per core");
    expect(outWords(machine({ out: "plan", held: "Claude's 5 h window is at 97 % until Nov 12 16:50" }))).toBe("GOG: Claude's 5 h window is at 97 % until Nov 12 16:50");
    expect(outWords(machine({ out: "budget", budget: "the budget for app is spent" }))).toBe("GOG: the budget for app is spent");
  });

  it("names the next machine, and says when none holds the repository", () => {
    expect(nextWords({ pick: machine({ working: 1 }) }, "claude")).toBe("next on GOG: load 0.2 per core, 1 agent working");
    expect(nextWords({}, "claude")).toBe("");
    expect(nextWords(undefined, "claude")).toBe("");
    expect(noRoomWords({})).toBe("no connected machine has a workspace open on a checkout of this repository");
    expect(noRoomWords({ others: [machine({ out: "hot", temp_c: 90 }), machine({ label: "Pi", out: "load", load_per_core: 3 })] })).toBe("no machine has room: GOG: hot at 90 °C; Pi: load 3.0 per core");
  });
});
